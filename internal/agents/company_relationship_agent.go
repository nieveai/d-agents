package agents

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

// CompanyRelationship defines the structure for the JSON output from the GenAI client.
type CompanyRelationship struct {
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
}

type CompanyRelationshipAgent struct {
	DbDriver neo4j.Driver
}

func NewCompanyRelationshipAgent() (*CompanyRelationshipAgent, error) {
	driver, err := database.GetNeo4jDriver()
	if err != nil {
		return nil, fmt.Errorf("failed to get Neo4j driver: %w", err)
	}
	return &CompanyRelationshipAgent{DbDriver: driver}, nil
}

const companyRelationshipSystemPrompt = `you are a stock analyst. plesae find all the companies that are related to the one mentioned in user message. please include all the important relationships such as vendors, customers, competitors, etc. the output should in json format. for example: [ { "name" : "nvidia", "relationship": "vendor"}, ... ]. a company may have multiple relationship. for example, it can be vendor as well as competitor.`

func (a *CompanyRelationshipAgent) DoWork(workload *pb.Workload, genAIClient m.GenAIClient) error {
	if workload == nil {
		return fmt.Errorf("workload is nil")
	}
	if genAIClient == nil {
		return fmt.Errorf("genAIClient is nil")
	}
	if workload.Name == "" {
		return fmt.Errorf("workload name (session name) is empty, which is required as a primary company node")
	}

	input := string(workload.Payload)
	// Pass the payload to the GenAI client to get the relationship JSON
	llmResponse, err := genAIClient.GenerateContentWithSystemPrompt(workload, input, companyRelationshipSystemPrompt)
	if err != nil {
		return fmt.Errorf("error generating content: %w", err)
	}

	// Extract the JSON part from the response
	jsonString := extractJSONArray(llmResponse)
	if jsonString == "" {
		return fmt.Errorf("no JSON array found in the LLM response")
	}

	var relationships []CompanyRelationship
	if err := json.Unmarshal([]byte(jsonString), &relationships); err != nil {
		return fmt.Errorf("failed to parse JSON from LLM response: %w", err)
	}

	// Process the relationships and update Neo4j
	summary, err := a.updateRelationshipsInNeo4j(workload.Name, relationships)
	if err != nil {
		return fmt.Errorf("failed to update Neo4j database: %w", err)
	}

	// Update the payload with the results
	newPayload := fmt.Sprintf("%s\n\n---\n\n%s\n\nProcessed Relationships:\n%s", input, llmResponse, summary)
	workload.Payload = []byte(newPayload)

	return nil
}


// extractJSONArray finds and extracts the first JSON array from a string.
func extractJSONArray(s string) string {
	re := regexp.MustCompile(`(?s)[\[].*[\]]`) // Corrected regex to properly match JSON arrays
	return re.FindString(s)
}

// sanitizeRelationshipType prepares a string to be used as a Neo4j relationship type.
func sanitizeRelationshipType(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, " ", "_")
	// Add any other sanitization rules if necessary
	return s
}

func (a *CompanyRelationshipAgent) updateRelationshipsInNeo4j(sessionName string, relationships []CompanyRelationship) (string, error) {
	session := a.DbDriver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	var summaryBuilder strings.Builder

	for _, rel := range relationships {
		otherCompany := rel.Name
		relationshipTypes := strings.Split(rel.Relationship, ",")

		for _, relType := range relationshipTypes {
			sanitizedRelType := sanitizeRelationshipType(relType)
			if sanitizedRelType == "" {
				continue
			}

			_, err := session.WriteTransaction(func(tx neo4j.Transaction) (interface{}, error) {
				query := `
					MERGE (c1:Company {name: $sessionName})
					MERGE (c2:Company {name: $otherCompany})
					MERGE (c2)-[r:%s]->(c1)`
				// Note: Relationship types cannot be parameterized directly in Cypher.
				// It's generally safe here as we are sanitizing the input string.
				finalQuery := fmt.Sprintf(query, sanitizedRelType)

				result, err := tx.Run(finalQuery, map[string]interface{}{
					"sessionName":  sessionName,
					"otherCompany": otherCompany,
				})
				if err != nil {
					return nil, err
				}
				return nil, result.Err()
			})

			if err != nil {
				errorMsg := fmt.Sprintf("Failed to add relationship: %s -[%s]-> %s. Error: %v\n", otherCompany, sanitizedRelType, sessionName, err)
				summaryBuilder.WriteString(errorMsg)
				// Decide if we should continue or return on first error. Continuing for now.
			} else {
				successMsg := fmt.Sprintf("Added relationship: %s -[%s]-> %s\n", otherCompany, sanitizedRelType, sessionName)
				summaryBuilder.WriteString(successMsg)
			}
		}
	}

	return summaryBuilder.String(), nil
}
