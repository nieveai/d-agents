package main

import (
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	localmcp "github.com/nieveai/d-agents/internal/mcp"
)

func main() {
	// Create a new MCP client.
	client := localmcp.NewClient()

	// Create a new stdio transport.
	transport := &mcp.StdioTransport{}

	// Connect to the server.
	session, err := localmcp.Connect(client, transport)
	if err != nil {
		log.Fatalf("failed to connect to MCP server: %v", err)
	}
	defer session.Close()

	// Get the server capabilities.
	capabilities := localmcp.GetServerCapabilities(session)

	// Print the capabilities.
	fmt.Printf("Server capabilities: %+v\n", capabilities)
}
