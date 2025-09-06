package agents

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

// ShoppingResult defines the structure for the JSON output from the GenAI client.
type ShoppingResult struct {
	Name   string  `json:"name"`
	Price  float64 `json:"price"`
	Source string  `json:"source"`
	URL    string  `json:"url"`
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

const shoppingSystemPromptTemplate = `you are a shopping assistant. from the provided HTML content, please find all products similar to "%s". extract the product name, price, source and product URL for each. the output should be a JSON array. for example: [ { "name" : "product name", "price": 12.34, "source": "amazon.com", "url": "http://amazon.com/product/123" }, ...]`

func (a *ShoppingAgent) DoWork(workload *pb.Workload, genAIClient m.GenAIClient) error {
	if workload == nil {
		return fmt.Errorf("workload is nil")
	}
	if genAIClient == nil {
		return fmt.Errorf("genAIClient is nil")
	}
	if workload.Name == "" {
		return fmt.Errorf("workload name (the product name) is empty")
	}

	input := string(workload.Payload)
	url := extractURL(input)

	var processedInput string
	if url != "" {
		htmlContent, err := getHTMLFromURL(url)
		if err != nil {
			return fmt.Errorf("failed to get HTML from URL %s: %w", url, err)
		}
		processedInput = htmlContent
	} else {
		processedInput = input
	}

	// Pass the payload to the GenAI client to get the shopping result JSON
	systemPrompt := fmt.Sprintf(shoppingSystemPromptTemplate, workload.Name)
	llmResponse, err := genAIClient.GenerateContentWithSystemPrompt(workload, processedInput, systemPrompt)
	if err != nil {
		return fmt.Errorf("error generating content: %w", err)
	}

	// Extract the JSON part from the response
	jsonString := extractJSONArray(llmResponse)

	if jsonString == "" {
		fmt.Printf("%s\n", llmResponse)
		return fmt.Errorf("no JSON array found in the LLM response")
	}

	var results []ShoppingResult
	if err := json.Unmarshal([]byte(jsonString), &results); err != nil {
		return fmt.Errorf("failed to parse JSON from LLM response: %w", err)
	}

	// Process the shopping results and update the database
	for _, result := range results {
		err = a.Db.InsertProduct(result.Name, result.Price, time.Now(), result.Source, result.URL)
		if err != nil {
			// Log the error and continue with the next product
			fmt.Printf("failed to insert product %s: %v\n", result.Name, err)
		}
	}

	return nil
}

