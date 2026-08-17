package main

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/counter/counterpbv1"
	"github.com/the-protobuf-project/runtime-go/mcpruntime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestSmokeCounterService verifies the streaming Count tool:
//  1. Start gRPC server with counter implementation
//  2. Create MCP server forwarding to gRPC
//  3. List tools — expect 1 (Count)
//  4. Call Count with to=3 and verify the final result
func TestSmokeCounterService(t *testing.T) {
	ctx := context.Background()

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer lis.Close()

	gs := grpc.NewServer()
	counterpbv1.RegisterCounterServiceServer(gs, newCounterServer())
	go func() { _ = gs.Serve(lis) }()
	defer gs.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial gRPC: %v", err)
	}
	defer conn.Close()

	client := counterpbv1.NewCounterServiceClient(conn)
	server := mcpruntime.NewMCPServer(&mcpruntime.MCPServerConfig{
		Name:    "smoke-counter",
		Version: "0.0.1",
	})
	counterpbv1.ForwardToCounterServiceMCPClient(server, client)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, serverTransport) }()

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "smoke-client",
		Version: "0.0.1",
	}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	toolsResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsResult.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolsResult.Tools))
	}
	if toolsResult.Tools[0].Name != "counter_service-count_v1" {
		t.Fatalf("expected tool counter_service-count_v1, got %s", toolsResult.Tools[0].Name)
	}

	countArgs, _ := json.Marshal(map[string]any{"to": 3})
	countResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "counter_service-count_v1",
		Arguments: json.RawMessage(countArgs),
	})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	text := extractText(countResult)
	if !strings.Contains(text, `"count":3`) && !strings.Contains(text, `"count": 3`) {
		t.Fatalf("expected count 3 in response, got: %s", text)
	}
	t.Logf("Count response: %s", text)
}

func extractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	b, _ := json.Marshal(result.Content[0])
	var block struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(b, &block)
	return block.Text
}
