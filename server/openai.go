package server

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmorganca/ollama/api"
	"golang.org/x/exp/slices"
)

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatCompletion struct {
	// Model is ignored, the model is specified in the environment variable OLLAMA_OPENAI_MODEL
	// TODO: should check if the model is in a known list of models?
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OpenAIDelta struct {
	Role    string  `json:"role,omitempty"`
	Content *string `json:"content,omitempty"`
}

type OpenAIChatCompletionResponseChoice struct {
	Index        int         `json:"index"`
	Delta        OpenAIDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionResponse struct {
	ID                string                               `json:"id"`
	Object            string                               `json:"object"`
	Created           int                                  `json:"created"`
	Model             string                               `json:"model"`
	SystemFingerprint string                               `json:"system_fingerprint"`
	Choices           []OpenAIChatCompletionResponseChoice `json:"choices"`
	Usage             OpenAIUsage                          `json:"usage,omitempty"`
}

func applyTemplate(modelTempate string, messages []OpenAIMessage) (string, error) {
	templ, err := template.New("").Parse(modelTempate)
	if err != nil {
		return "", err
	}

	nextInSequence := []string{""} // The valid next roles in the chat sequence for the same prompt
	if len(messages) > 0 {
		nextInSequence[0] = messages[0].Role
	}

	type Prompt struct {
		System    string
		Prompt    string
		Assistant string
	}
	prompt := Prompt{}
	var result strings.Builder

	for _, msg := range messages {
		if !slices.Contains(nextInSequence, msg.Role) {
			// end of sequence, generate the prompt
			if err := templ.Execute(&result, prompt); err != nil {
				return "", err
			}
			if prompt.Assistant != "" {
				result.WriteString("\n")
				result.WriteString(prompt.Assistant)
			}
			// Start the new prompt with the current message
			prompt = Prompt{}
		}

		switch msg.Role {
		case "system":
			prompt.System = msg.Content
			nextInSequence = []string{"user", "assistant"}
		case "user":
			prompt.Prompt = msg.Content
			nextInSequence = []string{"assistant"}
		case "assistant":
			prompt.Assistant = msg.Content
			nextInSequence = []string{""} // signals the end of the sequence
		default:
			return "", fmt.Errorf("unexpected role: %s", msg.Role)
		}
	}
	// Generate the final prompt
	if prompt.System != "" || prompt.Prompt != "" || prompt.Assistant != "" {
		if err := templ.Execute(&result, prompt); err != nil {
			return "", err
		}
		if prompt.Assistant != "" {
			result.WriteString("\n")
			result.WriteString(prompt.Assistant)
		}
	}
	return result.String(), nil
}

func ListOpenAIModelsHandler(c *gin.Context) {
	models, err := listModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	modelList := []OpenAIModel{}
	for _, m := range models {
		modelList = append(modelList, OpenAIModel{
			ID:      m.Name,
			Object:  "model",
			Created: int(m.ModifiedAt.Unix()),
			OwnedBy: "ollama",
		})
	}

	fmt.Println(modelList)

	c.JSON(http.StatusOK, OpenAIModelList{
		Object: "list",
		Data:   modelList,
	})
}

func ChatCompletions(c *gin.Context) {
	var req OpenAIChatCompletion
	err := c.ShouldBindJSON(&req)
	switch {
	case errors.Is(err, io.EOF):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing request body"})
		return
	case err != nil:
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenAI requests will specify models which are not available through Ollama, map them to a user defined model
	ollamaModel := os.Getenv("OLLAMA_OPENAI_MODEL")
	if len(ollamaModel) == 0 {
		ollamaModel = "llama2"
	}

	// Map OpenAI messages to prompts
	model, err := GetModel(ollamaModel)
	if err != nil {
		var pErr *fs.PathError
		if errors.As(err, &pErr) {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("model '%s' not found, try pulling it first", ollamaModel)})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	prompt, err := applyTemplate(model.Template, req.Messages)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Call generate and receive the channel with the responses
	ch, err := generate(c, api.GenerateRequest{
		Model:  ollamaModel,
		Prompt: prompt,
		Stream: &req.Stream,
		Raw:    true,
	})
	if err != nil {
		// TODO: translate to OpenAI error
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !req.Stream {
		// Accumulate responses for non-streaming request
		accumulatedResponse := ""
		var finalResp api.GenerateResponse
		for val := range ch {
			resp, ok := val.(api.GenerateResponse)
			if !ok {
				// If val is not of type GenerateResponse, send an error response
				c.JSON(http.StatusInternalServerError, gin.H{"error": "received value is not a GenerateResponse"})
				return
			}
			accumulatedResponse += resp.Response
			finalResp = resp
			if resp.Done {
				break
			}
		}
		// Send a single response with accumulated content
		id := fmt.Sprintf("chatcmpl-%d", rand.Intn(999))
		chatCompletionResponse := OpenAIChatCompletionResponse{
			ID:                id,
			Object:            "chat.completion",
			Created:           int(finalResp.CreatedAt.Unix()), // Converting time.Time to Unix timestamp
			Model:             ollamaModel,
			SystemFingerprint: "fp_ollama",
			Choices: []OpenAIChatCompletionResponseChoice{
				{
					Index: 0,
					Delta: OpenAIDelta{
						Role:    "assistant",
						Content: &accumulatedResponse,
					},
					FinishReason: func(done bool) *string {
						if done {
							reason := "stop"
							return &reason
						}
						return nil
					}(finalResp.Done),
				},
			},
			Usage: OpenAIUsage{
				// TODO
				PromptTokens:     10,
				CompletionTokens: 10,
				TotalTokens:      20,
			},
		}
		c.JSON(http.StatusOK, chatCompletionResponse)
		return
	}

	// Now, create the intermediate channel and transformation goroutine
	transformedCh := make(chan any)

	go func() {
		defer close(transformedCh)
		id := fmt.Sprintf("chatcmpl-%d", rand.Intn(999))
		emptyContent := ""
		predefinedResponse := OpenAIChatCompletionResponse{
			ID:                id,
			Object:            "chat.completion.chunk",
			Created:           int(time.Now().Unix()),
			Model:             ollamaModel,
			SystemFingerprint: "fp_ollama",
			Choices: []OpenAIChatCompletionResponseChoice{
				{
					Index: 0,
					Delta: OpenAIDelta{
						Role:    "assistant",
						Content: &emptyContent,
					},
				},
			},
		}
		transformedCh <- predefinedResponse
		for val := range ch {
			resp, ok := val.(api.GenerateResponse)
			if !ok {
				// If val is not of type GenerateResponse, send an error down the channel and exit
				transformedCh <- gin.H{"error": "received value is not a GenerateResponse"}
				return
			}

			// TODO: handle errors

			// Transform the GenerateResponse into OpenAIChatCompletionResponse
			chatCompletionResponse := OpenAIChatCompletionResponse{
				ID:                id,
				Object:            "chat.completion.chunk",
				Created:           int(resp.CreatedAt.Unix()), // Converting time.Time to Unix timestamp
				Model:             resp.Model,
				SystemFingerprint: "fp_ollama",
				Choices: []OpenAIChatCompletionResponseChoice{
					{
						Index: 0,
						Delta: OpenAIDelta{
							Content: &resp.Response,
						},
						FinishReason: func(done bool) *string {
							if done {
								reason := "stop"
								return &reason
							}
							return nil
						}(resp.Done),
					},
				},
			}
			transformedCh <- chatCompletionResponse
		}
	}()

	// Pass the transformed channel to streamResponse
	streamResponse(c, transformedCh)
}
