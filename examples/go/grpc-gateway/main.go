package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"github.com/the-protobuf-project/runtime-go/mcpruntime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	grpcAddr := "localhost:50051"
	if a := os.Getenv("GRPC_ADDR"); a != "" {
		grpcAddr = a
	}
	mcpAddr := ":8084"
	if a := os.Getenv("MCP_ADDR"); a != "" {
		mcpAddr = a
	}

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial gRPC: %v", err)
	}
	defer conn.Close()

	client := todopbv1.NewTodoServiceClient(conn)

	cfg := &mcpruntime.MCPServerConfig{
		Name:              "todo-mcp-grpc-gateway",
		Version:           "0.1.0",
		Transports:        []mcpruntime.Transport{mcpruntime.TransportStreamableHTTP},
		Addr:              mcpAddr,
		GeneratedBasePath: todopbv1.TodoServiceMCPDefaultBasePath,
	}

	if ep, err := mcpruntime.ServerEndpoint(cfg); err == nil {
		log.Printf("MCP gRPC gateway listening on %s (forwarding to gRPC at %s)", ep.URL, grpcAddr)
	}

	if err := mcpruntime.StartServer(context.Background(), cfg, func(s *mcp.Server) {
		todopbv1.ForwardToTodoServiceMCPClient(s, client)
	}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
