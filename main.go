package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"text/template"
	"time"

	"swift-codegen/llmutils"

	"github.com/goccy/go-yaml"
)

type Algorithm struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Prompts struct {
	Class          string `yaml:"class"`
	ClassFilename  string `yaml:"classFilename"`
	Module         string `yaml:"module"`
	ModuleFilename string `yaml:"moduleFilename"`
}

type CodeGenerationRequest struct {
	Algorithm1Name string `json:"algorithm1"`
	Algorithm1Desc string `json:"algorithm1Desc"`
	Algorithm2Name string `json:"algorithm2"`
	Algorithm2Desc string `json:"algorithm2Desc"`
}

var defaultBridgingHeaderContent = `// Use this file to import your target's public headers that you would like to expose to Swift.
#import "React/RCTBridgeModule.h"`

var (
	codellama *llmutils.LLMHandler
	codegemma *llmutils.LLMHandler
)

func main() {
	start := time.Now()
	totalIterations := 0
	defer func() {
		log.Printf("⏱️ Iterations done: %d; Execution time: %v\n", totalIterations, time.Since(start).Round(time.Second))
	}()

	host := flag.String("host", "http://localhost:11434", "LLM server host")
	limit := flag.Int("limit", 10, "Number of iterations to run")
	outputDir := flag.String("output", "output", "Output directory")
	projectName := flag.String("project", "CleanerApp", "Project name")
	flag.Parse()

	log.Println("🚀 Hello and welcome to the Code Generator! 🚀")

	var err error
	codellama, err = llmutils.CreateLLMHandler("codellama:13b-instruct", *host)
	codegemma, err = llmutils.CreateLLMHandler("codegemma:7b-instruct", *host)
	if err != nil {
		log.Fatalf("failed to create LLM handler: %v", err)
	}

	createDirectoryIfNotExists(*outputDir)
	createFileIfNotExists(*outputDir+"/"+*projectName+"-Bridging-Header.h", defaultBridgingHeaderContent)

	done := make(chan struct{})
	for i := 0; i < *limit; i++ {
		log.Printf("🔄 Starting iteration %d/%d\n", i+1, *limit)
		go func(iteration int) {
			defer recoverAndRestart(iteration, &i, done)
			if err := runIteration(*outputDir); err != nil {
				log.Printf("❌ Error during iteration %d: %v. Restarting...\n", iteration, err)
				i--
			}
		}(i + 1)
		<-done
		time.Sleep(time.Second)
		totalIterations++
	}
	close(done)
	log.Println("🏁 That's all Folks! 🏁")
}

func createDirectoryIfNotExists(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0755)
	}
}

func createFileIfNotExists(path, content string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte(content), 0644)
	}
}

func recoverAndRestart(iteration int, i *int, done chan struct{}) {
	if r := recover(); r != nil {
		log.Printf("❌ Panic recovered during iteration %d: %v. Restarting...\n", iteration, r)
		(*i)--
	}
	done <- struct{}{}
}

func runIteration(outputDir string) error {
	prompts, err := getPrompts()
	if err != nil {
		return err
	}

	class, _, err := codellama.Prompt(prompts.Class)
	if err != nil {
		return err
	}

	files := llmutils.SplitIntoFiles(class)

	for _, file := range files {
		if file.Name == "" {
			return fmt.Errorf("file name is empty")
		}
		classWithMarks := llmutils.AddObjcAnnotations(file.Source)
		err = writeToFile(file.Name, outputDir, classWithMarks)
		if err != nil {
			return err
		}

		module, _, err := codegemma.Prompt(prompts.Module + "\n\n" + classWithMarks)
		mfiles := llmutils.SplitIntoFiles(module)
		for _, mfile := range mfiles {
			if mfile.Name == "" {
				return fmt.Errorf("module file name is empty")
			}
			err = writeToFile(mfile.Name, outputDir, mfile.Source)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func readAlgorithms(path string) ([]Algorithm, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	var algs []Algorithm
	if err := json.NewDecoder(file).Decode(&algs); err != nil {
		return nil, fmt.Errorf("error decoding file: %w", err)
	}
	return algs, nil
}

func readPrompts(path string) (Prompts, error) {
	file, err := os.Open(path)
	var prompts Prompts
	if err != nil {
		return prompts, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	if err := yaml.NewDecoder(file).Decode(&prompts); err != nil {
		return prompts, fmt.Errorf("error decoding file: %w", err)
	}
	return prompts, nil
}

func getPrompts() (Prompts, error) {
	var prompts Prompts
	algs, err := readAlgorithms("algorithms.json")
	if err != nil {
		return prompts, err
	}
	rndAlg, rndAlg2 := getRandomAlgorithms(algs)
	logAlgorithmDetails(rndAlg, rndAlg2)

	promptData := CodeGenerationRequest{
		Algorithm1Name: rndAlg.Name,
		Algorithm1Desc: rndAlg.Description,
		Algorithm2Name: rndAlg2.Name,
		Algorithm2Desc: rndAlg2.Description,
	}

	promptTemplates, err := readPrompts("prompts.yaml")
	if err != nil {
		return prompts, err
	}
	values := reflect.ValueOf(promptTemplates)
	types := values.Type()
	p := reflect.ValueOf(&prompts).Elem()
	for i := 0; i < values.NumField(); i++ {
		field := types.Field(i).Name
		value := values.Field(i)
		tmpl, err := template.New(field).Parse(value.String())
		if err != nil {
			return prompts, err
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, promptData)
		if err != nil {
			return prompts, err
		}
		p.FieldByName(field).SetString(buf.String())
	}

	return prompts, nil
}

func getRandomAlgorithms(algs []Algorithm) (Algorithm, Algorithm) {
	rndAlg := algs[rand.Intn(len(algs))]
	rndAlg2 := algs[rand.Intn(len(algs))]
	return rndAlg, rndAlg2
}

func logAlgorithmDetails(rndAlg, rndAlg2 Algorithm) {
	log.Printf("🚗 Algorithm 1: %s (%s)\n", rndAlg.Name, rndAlg.Description)
	log.Printf("🚙 Algorithm 2: %s (%s)\n", rndAlg2.Name, rndAlg2.Description)
}

func ensureDirectoryExists(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

func writeToFile(path, outputDir, content string) error {
	if err := ensureDirectoryExists(outputDir + "/" + path); err != nil {
		return err
	}
	err := os.WriteFile(outputDir+"/"+path, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}
	log.Printf("📝 File %s written\n", path)
	return nil
}
