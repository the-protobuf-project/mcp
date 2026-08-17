package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"github.com/the-protobuf-project/runtime-go/mcpruntime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	transports := mcpruntime.ParseTransports("streamable-http")
	if t := os.Getenv("MCP_TRANSPORT"); t != "" {
		transports = mcpruntime.ParseTransports(t)
	}
	mcpAddr := ":8082"
	if a := os.Getenv("MCP_ADDR"); a != "" {
		mcpAddr = a
	}
	grpcAddr := ":50051"
	if a := os.Getenv("GRPC_ADDR"); a != "" {
		grpcAddr = a
	}

	srv := newTodoServer()

	// Start gRPC server in a goroutine.
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			log.Fatalf("gRPC listen: %v", err)
		}
		gs := grpc.NewServer()
		todopbv1.RegisterTodoServiceServer(gs, srv)
		reflection.Register(gs)
		log.Printf("gRPC server listening on %s (reflection enabled)", grpcAddr)
		if err := gs.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	// Start MCP server (blocks).
	cfg := &mcpruntime.MCPServerConfig{
		Name:       "todo-mcp-example",
		Version:    "0.1.0",
		Transports: transports,
		Addr:       mcpAddr,
		// Set the proto-derived path as the generated default
		GeneratedBasePath: todopbv1.TodoServiceMCPDefaultBasePath,
	}

	if ep, err := mcpruntime.ServerEndpoint(cfg); err == nil {
		log.Printf("MCP will listen on %s", ep.URL)
	}

	// ServeTodoServiceMCP registers all generated tools, prompts, resources, and app.
	if err := todopbv1.ServeTodoServiceMCP(context.Background(), srv, cfg); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
