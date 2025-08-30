package models

import (
	pb "github.com/nieveai/d-agents/proto"
)

// Agent struct remains for data representation
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// genAIClient interface for generative AI clients
type GenAIClient interface {
	GenerateContent(workload *pb.Workload, input string) (string, error)
}

// Agent interface for agents to implement
type AgentInterface interface {
	DoWork(workload *pb.Workload, genAIClient GenAIClient) error
}
