package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/the-protobuf-project/mcp/examples/proto/generated/go/todo/todopbv1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

// TestSmokeTodoService verifies the full pipeline:
//  1. Create an MCP server with the generated TodoService tools
//  2. Connect an in-memory MCP client
//  3. List tools — expect 5 (Create, Get, List, Update, Delete)
//  4. Call CreateTodo and verify the response
//  5. Call GetTodo with the created name and verify
//  6. Call ListTodos and verify the item appears
func TestSmokeTodoService(t *testing.T) {
	ctx := context.Background()

	// --- Server ---
	srv := newTodoServer()
	server := mcp.NewMCPServer(&mcp.MCPServerConfig{
		Name:    "smoke-test",
		Version: "0.0.1",
	})
	todopbv1.RegisterTodoServiceMCPHandler(server, srv)

	// --- In-memory transport ---
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()

	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, serverTransport) }()

	// --- Client ---
	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "smoke-client",
		Version: "0.0.1",
	}, &mcpsdk.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcpsdk.ElicitRequest) (*mcpsdk.ElicitResult, error) {
			return &mcpsdk.ElicitResult{Action: "accept", Content: map[string]any{"confirm": "yes"}}, nil
		},
	})
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	// 1) List tools
	toolsResult, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	t.Logf("Discovered %d tools:", len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		t.Logf("  - %s: %s", tool.Name, tool.Description)
	}
	if len(toolsResult.Tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(toolsResult.Tools))
	}

	// 2) Call CreateTodo
	createArgs, _ := json.Marshal(map[string]any{
		"parent":  "users/alice",
		"todo_id": "task-1",
		"todo": map[string]any{
			"title":       "Buy groceries",
			"description": "Milk, eggs, bread",
			"priority":    "PRIORITY_HIGH",
		},
	})
	createResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-create_todo_v1",
		Arguments: json.RawMessage(createArgs),
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	createText := extractText(createResult)
	t.Logf("CreateTodo response: %s", createText)

	if !strings.Contains(createText, "users/alice/todos/task-1") {
		t.Fatalf("expected resource name in response, got: %s", createText)
	}
	if !strings.Contains(createText, "Buy groceries") {
		t.Fatalf("expected title in response, got: %s", createText)
	}

	// 3) Call GetTodo
	getArgs, _ := json.Marshal(map[string]any{
		"name": "users/alice/todos/task-1",
	})
	getResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-get_todo_v1",
		Arguments: json.RawMessage(getArgs),
	})
	if err != nil {
		t.Fatalf("GetTodo: %v", err)
	}
	getText := extractText(getResult)
	t.Logf("GetTodo response: %s", getText)

	if !strings.Contains(getText, "Buy groceries") {
		t.Fatalf("GetTodo didn't return expected todo, got: %s", getText)
	}

	// 4) Call ListTodos
	listArgs, _ := json.Marshal(map[string]any{
		"parent": "users/alice",
	})
	listResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-list_todos_v1",
		Arguments: json.RawMessage(listArgs),
	})
	if err != nil {
		t.Fatalf("ListTodos: %v", err)
	}
	listText := extractText(listResult)
	t.Logf("ListTodos response: %s", listText)

	if !strings.Contains(listText, "task-1") {
		t.Fatalf("ListTodos didn't include created todo, got: %s", listText)
	}

	// 5) Call DeleteTodo
	deleteArgs, _ := json.Marshal(map[string]any{
		"name": "users/alice/todos/task-1",
	})
	_, err = session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-delete_todo_v1",
		Arguments: json.RawMessage(deleteArgs),
	})
	if err != nil {
		t.Fatalf("DeleteTodo: %v", err)
	}
	t.Log("DeleteTodo succeeded")

	// 6) Verify deleted — ListTodos should return empty
	listResult2, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-list_todos_v1",
		Arguments: json.RawMessage(listArgs),
	})
	if err != nil {
		t.Fatalf("ListTodos after delete: %v", err)
	}
	listText2 := extractText(listResult2)
	if strings.Contains(listText2, "task-1") {
		t.Fatalf("todo should be deleted, but still found in: %s", listText2)
	}
	t.Log("Verified todo was deleted")
}

func extractText(result *mcpsdk.CallToolResult) string {
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
