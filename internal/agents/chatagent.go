package agents

import (
	"fmt"

	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

type ChatAgent struct{}

func (a *ChatAgent) DoWork(workload *pb.Workload, genAIClient m.GenAIClient) error {
	if workload == nil {
		return fmt.Errorf("workload is nil")
	}
	if genAIClient == nil {
		return fmt.Errorf("genAIClient is nil")
	}

	// For ChatAgent, the input to the LLM is simply the payload.
	input := string(workload.Payload)

	responseText, err := genAIClient.GenerateContent(workload, input)
	if err != nil {
		return fmt.Errorf("error generating content: %w", err)
	}

	fmt.Printf("\n\n%s\n", responseText)

	newPayload := fmt.Sprintf("%s\n\n---\n\n%s", string(workload.Payload), responseText)
	workload.Payload = []byte(newPayload)

	return nil
}
