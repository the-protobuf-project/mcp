# Go Examples

MCP server examples in Go: **TodoService** (CRUD, prompts, elicitation) and **CounterService** (progress streaming).

## Prerequisites

- Go 1.25+
- `protoc-gen-mcp` installed (`go install github.com/the-protobuf-project/mcp/plugin/cmd/protoc-gen-mcp@latest`)
- Generated code already in `proto/generated/go/` (run `buf generate` from `examples/`)

## Structure

```
go/
├── http/          # TodoService — streamable-http + gRPC side-by-side
│   ├── main.go
│   ├── impl.go
│   └── smoke_test.go
├── stdio/         # TodoService — stdio transport (Claude Desktop, MCP Inspector)
│   ├── main.go
│   └── impl.go
├── sse/           # TodoService — SSE transport (legacy)
│   ├── main.go
│   └── impl.go
├── grpc-gateway/  # TodoService — gRPC-to-MCP gateway forwarding
│   └── main.go
└── counter/       # CounterService — progress streaming (streamable-http)
    ├── main.go
    └── impl.go
```

**TodoService** examples use an in-memory `todoServer` implementing both gRPC `TodoServiceServer` and MCP `TodoServiceMCPServer`. **CounterService** demonstrates server-streaming with `mcp.MCPProgress` for progress notifications.

## Running

### Streamable HTTP (+ gRPC)

```bash
cd examples/go/http
go run .
# gRPC  → [::]:50051 (reflection enabled)
# MCP   → 0.0.0.0:8080/todo/v1/todoservice/mcp
```

### Stdio

```bash
cd examples/go/stdio
go run .
# MCP communicates over stdin/stdout
# Logs go to stderr to avoid corrupting JSON-RPC
```

For MCP Inspector:

```bash
go build -o /tmp/todo-stdio ./examples/go/stdio
npx @modelcontextprotocol/inspector -- /tmp/todo-stdio
```

### SSE

```bash
cd examples/go/sse
go run .
# MCP → 0.0.0.0:8080/todo/v1/todoservice/mcp (SSE)
```

### gRPC Gateway

Forwards MCP requests to an upstream gRPC server:

```bash
cd examples/go/grpc-gateway
go run .
# Connects to gRPC at localhost:50051
# MCP → 0.0.0.0:8080
```

### Counter (Progress Streaming)

Demonstrates MCP progress via server-streaming. Uses `ForwardToCounterServiceMCPClient` to forward tool calls to a gRPC backend:

```bash
cd examples/go/counter
go run .
# gRPC  → [::]:50052 (reflection + health)
# MCP   → 0.0.0.0:8083/counter/v1/counterservice/mcp
# Health → /health
```

Use `progressToken` in `params._meta` when calling the Count tool to receive progress notifications. For long runs, set `MCP_REQUEST_MAX_TOTAL_TIMEOUT=300000` when using MCP Inspector.

## Testing

The `http/` example includes a smoke test that exercises the full CRUD pipeline over an in-memory MCP transport:

```bash
cd examples/go/http
go test -v
```

Test flow:
1. List tools (expects 5)
2. CreateTodo
3. GetTodo
4. ListTodos
5. DeleteTodo
6. Verify deletion

## Architecture

**TodoService** (`todo_service.pb.mcp.go`):
- `TodoServiceMCPServer` interface — one method per unary RPC
- `RegisterTodoServiceMCPHandler(s, impl, opts...)` — registers tools, prompts, resources
- `ServeTodoServiceMCP(impl, cfg)` — convenience function to start the server

**CounterService** (`counter_service.pb.mcp.go`):
- `CounterServiceMCPClient` interface — streaming Count RPC
- `ForwardToCounterServiceMCPClient(s, client, opts...)` — forwards tool calls to gRPC, streams progress
- Server-streaming with `mcp.MCPProgress` for progress notifications

The `runtime` package handles transport selection, multi-transport serving, header-to-metadata forwarding, and schema injection.
