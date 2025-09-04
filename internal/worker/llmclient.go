package worker

import (
	"context"
	"fmt"
	"log"

	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
	"github.com/openai/openai-go/v2"
	openai_option "github.com/openai/openai-go/v2/option"
	"google.golang.org/genai"
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

		if _, ok := llm.clients[model.ID]; ok {
			continue
		}

		var client interface{}
		var err error

		switch model.APISpec {
		case "gemini":
			client, err = genai.NewClient(ctx,
				&genai.ClientConfig{
					APIKey:  model.APIKey,
					Backend: genai.BackendGeminiAPI,
				})
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
			log.Printf("Error initializing client for provider %s: %v", model.ID, err)
			continue
		}

		if client != nil {
			llm.clients[model.ID] = client
			log.Printf("Initialized client for provider: %s", model.ID)
		}
	}
	return llm, nil
}

func (llm *LLMClient) GenerateContent(workload *pb.Workload, input string) (string, error) {
	return llm.GenerateContentWithSystemPrompt(workload, input, "")
}

func (llm *LLMClient) GenerateContentWithSystemPrompt(workload *pb.Workload, input string, system_prompt string) (string, error) {
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

	client, ok := llm.clients[model.ID]
	if !ok {
		return "", fmt.Errorf("llm client not found for model '%s'", model.ID)
	}

	var responseText string
	var err error

	// Use a type switch to handle different client types
	switch c := client.(type) {
	case *genai.Client:
		var fullInput string
		config := &genai.GenerateContentConfig{}
		if system_prompt != "" {
			config.SystemInstruction = &genai.Content{Parts: []*genai.Part{&genai.Part{Text: system_prompt}}}
		}
		config.Tools = []*genai.Tool{
			{GoogleSearch: &genai.GoogleSearch{}},
		}
		fullInput = input

		result, e := c.Models.GenerateContent(context.Background(), model.ModelID, genai.Text(fullInput), config)
		if e != nil {
			err = fmt.Errorf("error calling Gemini API: %s", e)
		} else {
			responseText = result.Text()
		}

	case *openai.Client:
		messages := []openai.ChatCompletionMessageParamUnion{}
		if system_prompt != "" {
			messages = append(messages, openai.SystemMessage(system_prompt))
		}
		messages = append(messages, openai.UserMessage(string(input)))
		// Use the specific model ID (e.g., "gpt-4o") for the API call
		resp, e := c.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    openai.ChatModel(model.ModelID),
		})

		if e != nil {
			err = fmt.Errorf("error calling OpenAI API: %s", e)
		} else {
			responseText = resp.Choices[0].Message.Content
		}
	default:
		err = fmt.Errorf("unknown client type for model '%s'", model.ID)
	}

	if err != nil {
		return "", err
	}

	return responseText, nil
}
