package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/counter/counterpbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

// TestRegisterCounterServiceMCPHandler verifies the in-process (Register) path
// with a streaming tool using the streamable-HTTP transport:
//  1. Build MCP server using RegisterCounterServiceMCPHandler — no gRPC dial
//  2. Serve it over a local httptest.Server
//  3. List tools — expect the Count tool to be present
//  4. Call Count with to=3 and a progressToken
//  5. Handler returns {"status":"started"} immediately (non-blocking)
//  6. Background goroutine sends progress notifications over the SSE stream
//  7. Final notification (progress=1.0) contains the CountResponse JSON
func TestRegisterCounterServiceMCPHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mcpServer := mcp.NewMCPServer(&mcp.MCPServerConfig{
		Name:    "register-smoke",
		Version: "0.0.1",
	})

	// counterServer implements CounterServiceMCPServer because
	// InProcessServerStream[*CountStreamChunk] satisfies CounterService_CountServer.
	counterpbv1.RegisterCounterServiceMCPHandler(mcpServer, newCounterServer())

	// Use the streamable-HTTP transport so that detached-context notifications
	// (sent after the tool response is returned) are routed to the SSE stream.
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return mcpServer
	}, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	var (
		mu             sync.Mutex
		progressNotifs []mcpsdk.ProgressNotificationParams
		gotFinal       = make(chan struct{})
		closeOnce      sync.Once
	)

	mcpClient := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "register-smoke-client",
		Version: "0.0.1",
	}, &mcpsdk.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcpsdk.ProgressNotificationClientRequest) {
			mu.Lock()
			progressNotifs = append(progressNotifs, *req.Params)
			// SendDoneProgress sets Total=1.0 as the sentinel for completion.
			isFinal := req.Params.Total == 1.0
			mu.Unlock()
			if isFinal {
				closeOnce.Do(func() { close(gotFinal) })
			}
		},
	})

	session, err := mcpClient.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp",
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// 1. Verify Count tool is listed.
	toolsResult, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == "counter_service-count_v1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tool counter_service-count_v1 not found; tools: %v", toolsResult.Tools)
	}
	t.Logf("Tool listed OK: counter_service-count_v1")

	// 2. Call Count(to=3) with progressToken — expect immediate {"status":"started"}.
	// Meta must be pre-initialized; SetProgressToken doesn't call SetMeta when nil.
	countArgs, _ := json.Marshal(map[string]any{"to": 3})
	callParams := &mcpsdk.CallToolParams{
		Name:      "counter_service-count_v1",
		Arguments: json.RawMessage(countArgs),
		Meta:      mcpsdk.Meta{},
	}
	callParams.SetProgressToken("test-progress-token")

	callResult, err := session.CallTool(ctx, callParams)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := extractText(callResult)
	if text != `{"status":"started"}` {
		t.Fatalf("expected immediate {\"status\":\"started\"}, got: %s", text)
	}
	t.Logf("Immediate return OK: %s", text)

	// 3. Wait for the final progress notification (progress=1.0).
	select {
	case <-gotFinal:
		// success
	case <-ctx.Done():
		mu.Lock()
		count := len(progressNotifs)
		mu.Unlock()
		t.Fatalf("timed out waiting for final progress notification; received %d so far", count)
	}

	// 4. Verify final notification carries the CountResponse JSON.
	mu.Lock()
	notifs := append([]mcpsdk.ProgressNotificationParams{}, progressNotifs...)
	mu.Unlock()

	var finalMsg string
	for _, n := range notifs {
		// SendDoneProgress uses Total=1.0 as the completion sentinel.
		if n.Total == 1.0 {
			finalMsg = n.Message
			break
		}
	}
	if !strings.Contains(finalMsg, `"count"`) {
		t.Fatalf("final progress message missing 'count' field: %s", finalMsg)
	}

	// 5. Verify at least one intermediate notification (Total > 1.0) was
	// received. The Register path now forwards the progressToken as incoming gRPC
	// metadata so the counter implementation emits per-step progress chunks.
	hasIntermediate := false
	for _, n := range notifs {
		if n.Total > 1.0 {
			hasIntermediate = true
			break
		}
	}
	if !hasIntermediate {
		t.Fatalf("expected at least one intermediate progress notification (Total > 1.0); got %v", notifs)
	}

	t.Logf("Progress notifications received: %d", len(notifs))
	t.Logf("Final result notification: %s", finalMsg)
}
