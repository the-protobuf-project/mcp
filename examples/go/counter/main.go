package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/counter/counterpbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	transports := mcp.ParseTransports("streamable-http")
	if t := os.Getenv("MCP_TRANSPORT"); t != "" {
		transports = mcp.ParseTransports(t)
	}
	mcpAddr := ":8083"
	if a := os.Getenv("MCP_ADDR"); a != "" {
		mcpAddr = a
	}
	grpcAddr := ":50052"
	if a := os.Getenv("GRPC_ADDR"); a != "" {
		grpcAddr = a
	}

	srv := newCounterServer()

	// Start gRPC server in a goroutine.
	go func() {
		lis, err := net.Listen("tcp", grpcAddr)
		if err != nil {
			log.Fatalf("gRPC listen: %v", err)
		}
		gs := grpc.NewServer()
		healthcheck := health.NewServer()
		healthcheck.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthgrpc.RegisterHealthServer(gs, healthcheck)
		counterpbv1.RegisterCounterServiceServer(gs, srv)
		reflection.Register(gs)
		log.Printf("gRPC server listening on %s (reflection + health enabled)", grpcAddr)
		if err := gs.Serve(lis); err != nil {
			log.Fatalf("gRPC serve: %v", err)
		}
	}()

	// Allow gRPC server to start before dialing.
	time.Sleep(200 * time.Millisecond)

	// Connect MCP server to gRPC backend and forward tool calls.
	grpcTarget := "localhost" + grpcAddr
	conn, err := grpc.NewClient(grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial gRPC: %v", err)
	}
	defer conn.Close()

	client := counterpbv1.NewCounterServiceClient(conn)

	cfg := &mcp.MCPServerConfig{
		Name:              "counter-mcp-example",
		Version:           "0.1.0",
		Transports:        transports,
		Addr:              mcpAddr,
		GeneratedBasePath: counterpbv1.CounterServiceMCPDefaultBasePath,
		HealthCheckPath:   "/health",
		HealthCheckConn:   conn,
	}

	if ep, err := mcp.ServerEndpoint(cfg); err == nil {
		log.Printf("MCP will listen on %s (forwarding Count to gRPC at %s, /health probes gRPC)", ep.URL, grpcAddr)
	}

	if err := mcp.StartServer(context.Background(), cfg, func(s *mcpsdk.Server) {
		counterpbv1.ForwardToCounterServiceMCPClient(s, client)
	}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
