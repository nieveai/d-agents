package main

import (
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nieveai/d-agents/internal/mcp"
)

func main() {
	// Create a new MCP client.
	client, err := mcp.NewClient()
	if err != nil {
		log.Fatalf("failed to create MCP client: %v", err)
	}

	// Create a new stdio transport.
	transport := &mcp.StdioTransport{}

	// Connect to the server.
	session, err := mcp.Connect(client, transport)
	if err != nil {
		log.Fatalf("failed to connect to MCP server: %v", err)
	}
	defer session.Close()

	// Get the server capabilities.
	capabilities := mcp.GetServerCapabilities(session)

	// Print the capabilities.
	fmt.Printf("Server capabilities: %+v\n", capabilities)
}

