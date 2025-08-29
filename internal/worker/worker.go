package worker

import (
	"context"
	"log"

	"github.com/nieveai/d-agents/internal/agents"
	"github.com/nieveai/d-agents/internal/database"
	m "github.com/nieveai/d-agents/internal/models"
	pb "github.com/nieveai/d-agents/proto"
)

var llmClient *LLMClient
var db database.Datastore

func Init(ctx context.Context, models []*m.Model, database_conn database.Datastore) error {
	var err error
	llmClient, err = NewLLMClient(ctx, models)
	if err != nil {
		return err
	}
	db = database_conn
	return nil
}

func ProcessWorkload(workload *pb.Workload) {
	var agent m.AgentInterface

	switch workload.AgentType {
	case "ChatAgent":
		agent = &agents.ChatAgent{}
	default:
		log.Printf("Unknown agent type: %s", workload.AgentType)
		return
	}

	err := agent.DoWork(workload, llmClient)
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
