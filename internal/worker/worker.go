package worker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
	"github.com/openai/openai-go/v2"
	"google.golang.org/api/option"
	openai_option "github.com/openai/openai-go/v2/option"
)

// clients stores the initialized client for each provider.
var clients map[string]interface{}

// modelInfo stores model details keyed by model ID.
var modelInfo map[string]*m.Model
var db database.Datastore

func Init(ctx context.Context, models []*m.Model, database_conn database.Datastore) error {
	db = database_conn
	clients = make(map[string]interface{})
	modelInfo = make(map[string]*m.Model)

	for _, model := range models {
		// Store model details by its unique ID
		modelInfo[model.ID] = model

		// If a client for this provider is already initialized, skip
		if _, ok := clients[model.Provider]; ok {
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
			continue // Skip to the next model
		}

		if err != nil {
			log.Printf("Error initializing client for provider %s: %v", model.Provider, err)
			continue // Skip to the next model
		}

		if client != nil {
			clients[model.Provider] = client
			log.Printf("Initialized client for provider: %s", model.Provider)
		}
	}
	return nil
}

func ProcessWorkload(workload *pb.Workload) {
	if len(workload.Models) == 0 {
		log.Println("Error: workload has no models specified")
		return
	}
	// For now, just process the first model in the list.
	modelID := workload.Models[0]
	log.Printf("Processing workload for model ID: %s", modelID)

	model, ok := modelInfo[modelID]
	if !ok {
		log.Printf("Error: model information not found for model ID '%s'", modelID)
		return
	}

	client, ok := clients[model.Provider]
	if !ok {
		log.Printf("Error: client not found for provider '%s'", model.Provider)
		return
	}

	// Use a type switch to handle different client types
	switch c := client.(type) {
	case *genai.Client:
		// Use the specific model ID (e.g., "gemini-pro") for the API call
		gm := c.GenerativeModel(model.ModelID)
		resp, err := gm.GenerateContent(context.Background(), genai.Text(workload.Payload))
		if err != nil {
			log.Printf("Error calling Gemini API: %s", err)
			return
		}
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
		responseText := fullResponse.String()
		log.Printf("Gemini API response: %s", responseText)
		appendPayloadAndSave(workload.Id, responseText)

	case *openai.Client:
		// Use the specific model ID (e.g., "gpt-4o") for the API call
		resp, err := c.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(string(workload.Payload)),
			},
			Model: openai.ChatModel(model.ModelID),
		})

		if err != nil {
			log.Printf("Error calling OpenAI API: %s", err)
			return
		}

		responseText := resp.Choices[0].Message.Content
		log.Printf("OpenAI API response: %s", responseText)
		appendPayloadAndSave(workload.Id, responseText)
	default:
		log.Printf("Error: unknown client type for provider '%s'", model.Provider)
	}
}

func appendPayloadAndSave(workloadId string, responseText string) {
	session, err := db.GetSession(workloadId)
	if err != nil {
		log.Printf("Error getting session %s from db: %s", workloadId, err)
		return
	}

	newPayload := fmt.Sprintf("%s\n\n---\n\n%s", string(session.Payload), responseText)
	session.Payload = []byte(newPayload)

	if err := db.AddSession(session); err != nil {
		log.Printf("Error saving updated session %s to db: %s", workloadId, err)
	}
}
