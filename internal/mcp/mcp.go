package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewClient() (*mcp.Client, error) {
	// This is a placeholder for the actual MCP gateway address.
	// You should replace this with the actual address.
	gatewayAddress := "mcp.example.com:443"

	// Create a new MCP client.
	client, err := mcp.NewClient(gatewayAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client, nil
}

func GetModel(client *mcp.Client, modelID string) (string, error) {
	// Get the model from the MCP.
	model, err := client.GetModel(context.Background(), modelID)
	if err != nil {
		return "", fmt.Errorf("failed to get model from MCP: %w", err)
	}

	return model.Content, nil
}
