package worker

import (
	"context"
	"log"
	"sync"

	"github.com/nieveai/d-agents/internal/agents"
	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

var (
	llmClient *LLMClient
	db        database.Datastore
	llmMutex  = &sync.RWMutex{}
)

func Init(ctx context.Context, models []*m.Model, database_conn database.Datastore) error {
	db = database_conn
	return ReinitializeLLMClient(ctx, models)
}

func ReinitializeLLMClient(ctx context.Context, models []*m.Model) error {
	llmMutex.Lock()
	defer llmMutex.Unlock()

	var err error
	llmClient, err = NewLLMClient(ctx, models)
	if err != nil {
		return err
	}
	log.Println("LLM Client reinitialized with updated models.")
	return nil
}

func ProcessWorkload(workload *pb.Workload) {
	var agent m.AgentInterface
	var err error

	switch workload.AgentType {
	case "ChatAgent":
		agent = &agents.ChatAgent{}
	case "CompanyRelationshipAgent":
		agent, err = agents.NewCompanyRelationshipAgent()
		if err != nil {
			log.Printf("Error creating CompanyRelationshipAgent: %s", err)
			return
		}
	case "ShoppingAgent":
		agent, err = agents.NewShoppingAgent()
		if err != nil {
			log.Printf("Error creating ShoppingAgent: %s", err)
			return
		}
	default:
		log.Printf("Unknown agent type: %s", workload.AgentType)
		return
	}

	llmMutex.RLock()
	client := llmClient
	llmMutex.RUnlock()

	err = agent.DoWork(workload, client)
	if err != nil {
		log.Printf("Error processing workload: %s", err)
		// Optionally, update workload status to FAILED
		return
	}

	session, err := db.GetSession(workload.Id)
	if err != nil {
		log.Printf("Error getting session %s from db: %s", workload.Id, err)
		return
	}

	session.Payload = workload.Payload
	session.Status = pb.WorkloadStatus_COMPLETED

	if err := db.AddSession(session); err != nil {
		log.Printf("Error saving updated session %s to db: %s", workload.Id, err)
	}
}

