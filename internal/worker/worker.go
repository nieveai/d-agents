package worker

import (
	"context"
	"fmt"
	"log"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
	"github.com/openai/openai-go/v2"
	openai_option "github.com/openai/openai-go/v2/option"

	"github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

var genaiClient *genai.GenerativeModel
var openaiClient *openai.Client
var currentProvider string
var currentModel string

func Init(ctx context.Context, model *models.Model) error {
	currentProvider = model.Provider
	currentModel = model.ModelID
	switch model.Provider {
	case "gemini":
		client, err := genai.NewClient(ctx, option.WithAPIKey(model.APIKey))
		if err != nil {
			return err
		}
		genaiClient = client.GenerativeModel(model.ModelID)
	case "openai":
		client := openai.NewClient(openai_option.WithAPIKey("model.APIKey"))
		openaiClient = &client
	default:
		return fmt.Errorf("unknown model provider: %s", model.Provider)
	}
	return nil
}

func ProcessWorkload(workload *pb.Workload) {
	log.Printf("Processing workload in worker.go: %s", workload.Type)

	switch currentProvider {
	case "gemini":
		if genaiClient == nil {
			log.Println("Gemini client not initialized")
			return
		}

		resp, err := genaiClient.GenerateContent(context.Background(), genai.Text(workload.Payload))
		if err != nil {
			log.Printf("Error calling Gemini API: %s", err)
			return
		}

		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if txt, ok := part.(genai.Text); ok {
						log.Printf("Gemini API response: %s", txt)
					}
				}
			}
		}
	case "openai":
		if openaiClient == nil {
			log.Println("OpenAI client not initialized")
			return
		}
		resp, err := openaiClient.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage("Say this is a test"),
			},
			Model: openai.ChatModelGPT4o,
		})
		/* resp, err := openaiClient.Chat.Completions.New(context.Background(), openai.CompletionNewParams{
			Model:    openai.ChatModel(currentModel),
			Messages: []openai.MessageParam{openai.UserMessage(string(workload.Payload))},
		}) */

		if err != nil {
			log.Printf("Error calling OpenAI API: %s", err)
			return
		}

		log.Printf("OpenAI API response: %s", resp.Choices[0].Message.Content)
	}
}
