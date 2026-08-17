package main

import (
	"context"
	"log"
	"os"

	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

func main() {
	addr := ":8083"
	if a := os.Getenv("MCP_ADDR"); a != "" {
		addr = a
	}

	srv := newTodoServer()

	cfg := &mcp.MCPServerConfig{
		Name:              "todo-mcp-sse",
		Version:           "0.1.0",
		Transports:        []mcp.Transport{mcp.TransportSSE},
		Addr:              addr,
		GeneratedBasePath: todopbv1.TodoServiceMCPDefaultBasePath,
	}

	if ep, err := mcp.ServerEndpoint(cfg); err == nil {
		log.Printf("MCP SSE server listening on %s", ep.URL)
	}

	if err := todopbv1.ServeTodoServiceMCP(context.Background(), srv, cfg); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
