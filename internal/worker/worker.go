package worker

import (
	"context"
	"log"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	pb "github.com/nieveai/d-agents/proto"
)

var genaiClient *genai.GenerativeModel

func Init(ctx context.Context, apiKey string) error {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return err
	}
	genaiClient = client.GenerativeModel("gemini-2.5-flash")
	return nil
}

func ProcessWorkload(workload *pb.Workload) {
	log.Printf("Processing workload in worker.go: %s", workload.Type)

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
				if txt, ok := part.(genai.Text);
					ok {
						log.Printf("Gemini API response: %s", txt)
					}
			}
		}
	}
}
