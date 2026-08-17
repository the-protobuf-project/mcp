package main

import (
	"context"
	"log"

	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

func main() {
	srv := newTodoServer()

	cfg := &mcp.MCPServerConfig{
		Name:              "todo-mcp-stdio",
		Version:           "0.1.0",
		Transports:        []mcp.Transport{mcp.TransportStdio},
		GeneratedBasePath: todopbv1.TodoServiceMCPDefaultBasePath,
	}

	_ = log.Default() // logs go to stderr inside stdio transport

	if err := todopbv1.ServeTodoServiceMCP(context.Background(), srv, cfg); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
