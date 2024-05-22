package llmutils

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

type LLMHandler struct {
	llm *ollama.LLM
}

var ctx context.Context

func init() {
	ctx = context.Background()
}

func CreateLLMHandler(model, host string) (*LLMHandler, error) {
	llm, err := ollama.New(ollama.WithModel(model), ollama.WithServerURL(host))
	if err != nil {
		return nil, fmt.Errorf("failed to create Llama: %w", err)
	}
	return &LLMHandler{llm: llm}, nil
}

func (h *LLMHandler) Prompt(prompt string) (resultString, fileName string, err error) {

	chunkCounter := 0
	linebreakCounter := 0
	resultString, err = h.llm.Call(
		ctx,
		prompt,
		// llms.WithJSONMode(),
		llms.WithTemperature(0.9),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			chunkCounter++
			linebreakCounter += strings.Count(string(chunk), "\n")
			if chunkCounter > 10 {
				fmt.Printf("\r")
				for i := 0; i < linebreakCounter; i++ {
					fmt.Printf("\033[K\033[A")
				}
				chunkCounter = 0
				linebreakCounter = 0
			}
			fmt.Printf("%s", chunk)
			return nil
		}),
	)
	if err != nil {
		return "", "", fmt.Errorf("error generating code: %w", err)
	}

	fileName = findFilename(resultString)
	return
}

func stripAnnotation(input string) string {
	var output string
	pattern := regexp.MustCompile("```[a-zA-Z-_]{0,10}\n(.*?)```")
	matches := pattern.FindAllStringSubmatch(input, -1)
	if len(matches) > 0 {
		output = matches[0][1]
	} else {
		output = input
	}
	output = strings.Trim(output, "\n`* ")
	pattern = regexp.MustCompile("(?s)(.*)```.*")
	output = pattern.ReplaceAllString(output, `$1`)
	return output
}

func findFilename(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "//") && (len(lines[0]) > 3) && strings.Contains(lines[0], ".") {
		return lines[0][3:]
	}
	return ""
}

type file struct {
	Name   string
	Source string
}

func SplitIntoFiles(input string) []file {
	var files []file
	lines := strings.Split(input, "\n")
	lineNums := []int{}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//") && len(line) > 3 && (strings.Contains(line, ".swift") || strings.Contains(line, ".m") || strings.Contains(line, ".h")) {
			files = append(files, file{Name: line[3:], Source: ""})
			lineNums = append(lineNums, i)
		}
	}
	for i, f := range files {
		if i == len(files)-1 {
			f.Source = strings.Join(lines[lineNums[i]:], "\n")
		} else {
			f.Source = strings.Join(lines[lineNums[i]:lineNums[i+1]], "\n")
		}
		f.Source = stripAnnotation(f.Source)
		files[i] = f
	}

	if len(files) == 0 {
		files = append(files, file{Name: "", Source: input})
	}

	return files
}

func AddObjcAnnotations(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "class") {
			spaces := countLeadingSpaces(line)
			re := regexp.MustCompile(`class ([a-zA-Z0-9_]+)`)
			matches := re.FindStringSubmatch(trimmed)
			if len(matches) == 0 {
				continue
			}
			className := matches[1]
			lines[i] = strings.Repeat(" ", spaces) + "@objc(" + className + ")\n" + line
		}
		if strings.HasPrefix(trimmed, "func") {
			spaces := countLeadingSpaces(line)
			lines[i] = strings.Repeat(" ", spaces) + "@objc\n" + strings.Repeat(" ", spaces) + "@ReactMethod\n" + line
		}
	}
	return strings.Join(lines, "\n")
}

func countLeadingSpaces(str string) int {
	count := 0
	for _, char := range str {
		if !unicode.IsSpace(char) {
			break
		}
		count++
	}
	return count
}
