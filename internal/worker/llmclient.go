package worker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
	"github.com/openai/openai-go/v2"
	openai_option "github.com/openai/openai-go/v2/option"
	"google.golang.org/api/option"
)

type LLMClient struct {
	clients   map[string]interface{}
	modelInfo map[string]*m.Model
}

func NewLLMClient(ctx context.Context, models []*m.Model) (*LLMClient, error) {
	llm := &LLMClient{
		clients:   make(map[string]interface{}),
		modelInfo: make(map[string]*m.Model),
	}

	for _, model := range models {
		llm.modelInfo[model.ID] = model

		if _, ok := llm.clients[model.Provider]; ok {
			continue
		}

		var client interface{}
		var err error

		switch model.APISpec {
		case "gemini":
			c, e := genai.NewClient(ctx, option.WithAPIKey(model.APIKey))
			if e != nil {
				err = e
			} else {
				client = c
			}
		case "openai":
			opts := []openai_option.RequestOption{openai_option.WithAPIKey(model.APIKey)}
			if model.APIURL != "" {
				opts = append(opts, openai_option.WithBaseURL(model.APIURL))
			}
			c := openai.NewClient(opts...)
			client = &c
		default:
			log.Printf("Unknown or unspecified API spec for model %s: '%s'", model.ID, model.APISpec)
			continue
		}

		if err != nil {
			log.Printf("Error initializing client for provider %s: %v", model.Provider, err)
			continue
		}

		if client != nil {
			llm.clients[model.Provider] = client
			log.Printf("Initialized client for provider: %s", model.Provider)
		}
	}
	return llm, nil
}

func (llm *LLMClient) GenerateContent(workload *pb.Workload, input string) (string, error) {
	if len(workload.Models) == 0 {
		return "", fmt.Errorf("workload has no models specified")
	}
	// For now, just process the first model in the list.
	modelID := workload.Models[0]
	log.Printf("Processing workload for model ID: %s", modelID)

	model, ok := llm.modelInfo[modelID]
	if !ok {
		return "", fmt.Errorf("model information not found for model ID '%s'", modelID)
	}

	client, ok := llm.clients[model.Provider]
	if !ok {
		return "", fmt.Errorf("client not found for provider '%s'", model.Provider)
	}

	var responseText string
	var err error

	// Use a type switch to handle different client types
	switch c := client.(type) {
	case *genai.Client:
		// Use the specific model ID (e.g., "gemini-pro") for the API call
		gm := c.GenerativeModel(model.ModelID)
		resp, e := gm.GenerateContent(context.Background(), genai.Text(input))
		if e != nil {
			err = fmt.Errorf("error calling Gemini API: %s", e)
		} else {
			var fullResponse strings.Builder
			for _, cand := range resp.Candidates {
				if cand.Content != nil {
					for _, part := range cand.Content.Parts {
						if txt, ok := part.(genai.Text); ok {
							fullResponse.WriteString(string(txt))
						}
					}
				}
			}
			responseText = fullResponse.String()
		}

	case *openai.Client:
		// Use the specific model ID (e.g., "gpt-4o") for the API call
		resp, e := c.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(string(input)),
			},
			Model: openai.ChatModel(model.ModelID),
		})

		if e != nil {
			err = fmt.Errorf("error calling OpenAI API: %s", e)
		} else {
			responseText = resp.Choices[0].Message.Content
		}
	default:
		err = fmt.Errorf("unknown client type for provider '%s'", model.Provider)
	}

	if err != nil {
		return "", err
	}

	return responseText, nil
}
