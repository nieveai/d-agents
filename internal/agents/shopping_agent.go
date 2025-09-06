package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

// ShoppingResult defines the structure for the JSON output from the GenAI client.
type ShoppingResult struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Source string `json:"source"`
}

type ShoppingAgent struct {
	Db *database.ShoppingDB
}

func NewShoppingAgent() (*ShoppingAgent, error) {
	db, err := database.NewShoppingDB()
	if err != nil {
		return nil, fmt.Errorf("failed to get shopping db: %w", err)
	}
	return &ShoppingAgent{Db: db}, nil
}

const shoppingSystemPrompt = `you are a shopping assistant. please find the price of the product mentioned in user message. the output should in json format. for example: { "name" : "product name", "price": 12.34, "source": "amazon.com" }.`

func (a *ShoppingAgent) DoWork(workload *pb.Workload, genAIClient m.GenAIClient) error {
	if workload == nil {
		return fmt.Errorf("workload is nil")
	}
	if genAIClient == nil {
		return fmt.Errorf("genAIClient is nil")
	}

	input := string(workload.Payload)
	// Pass the payload to the GenAI client to get the shopping result JSON
	llmResponse, err := genAIClient.GenerateContentWithSystemPrompt(workload, input, shoppingSystemPrompt)
	if err != nil {
		return fmt.Errorf("error generating content: %w", err)
	}

	// Extract the JSON part from the response
	jsonString := extractJSONObject(llmResponse)
	if jsonString == "" {
		return fmt.Errorf("no JSON object found in the LLM response")
	}

	var result ShoppingResult
	if err := json.Unmarshal([]byte(jsonString), &result); err != nil {
		return fmt.Errorf("failed to parse JSON from LLM response: %w", err)
	}

	// Process the shopping result and update the database
	err = a.Db.InsertProduct(result.Name, result.Price, time.Now(), result.Source)
	if err != nil {
		return fmt.Errorf("failed to update shopping database: %w", err)
	}

	return nil
}

// extractJSONObject finds and extracts the first JSON object from a string.
func extractJSONObject(s string) string {
	re := regexp.MustCompile(`(?s){.*}`)
	return re.FindString(s)
}