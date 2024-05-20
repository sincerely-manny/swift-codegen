package llmutils

import (
	"context"
	"fmt"
	"strings"

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

	resultString = stripAnnotation(resultString)
	resultString = strings.Trim(resultString, "\n`*")
	fileName = findFilename(resultString)
	return
}

func stripAnnotation(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "```") {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}

func findFilename(input string) string {
	lines := strings.Split(input, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "//") && (len(lines[0]) > 3) && strings.Contains(lines[0], ".") {
		return lines[0][3:]
	}
	return ""
}
