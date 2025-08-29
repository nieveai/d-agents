package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewClient() (*mcp.Client, error) {
	// Create a new MCP client.
	client, err := mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return client, nil
}

func Connect(client *mcp.Client, transport mcp.Transport) (*mcp.ClientSession, error) {
	// Connect to the server.
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	return session, nil
}

func GetServerCapabilities(session *mcp.ClientSession) *mcp.ServerCapabilities {
	return session.InitializeResult().ServerCapabilities
}


