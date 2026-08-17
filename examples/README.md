# Examples

End-to-end examples demonstrating how to expose gRPC services as MCP servers in Go, Python, and Rust using `protoc-gen-mcp`.

## Overview

The examples include two proto services:

| Service         | Proto Path           | Description                                      |
| --------------- | -------------------- | ------------------------------------------------ |
| **TodoService** | `proto/todo/v1/`      | CRUD, prompts, elicitation, resources            |
| **CounterService** | `proto/counter/v1/` | Server-streaming with MCP progress notifications |

All languages share the same proto definitions and produce identical MCP tool surfaces. Each language example includes separate entrypoints for every supported transport:

| Transport | Description | Default Port |
| ------------------- | ----------------------------------------- | ------------ |
| `streamable-http` | Modern HTTP-based MCP transport | 8082 |
| `stdio` | Stdin/stdout for CLI tools (Claude Desktop) | — |
| `sse` | Server-Sent Events (legacy 2024-11-05 spec) | 8083 |

## Proto Definitions

Protos live in `proto/todo/v1/` and `proto/counter/v1/`, importing MCP annotations from the published BSR module:

```yaml
# buf.yaml
deps:
  - buf.build/the-protobuf-project/mcp
```

**TodoService** (`proto/todo/v1/todo_service.proto`):

```protobuf
import "mcp/protobuf/annotations.proto";

service TodoService {
  option (mcp.service) = {
    app: { name: "Todo App" version: "1.0.0" }
  };

  rpc CreateTodo(CreateTodoRequest) returns (Todo) {
    option (mcp.tool) = { ... };
    option (mcp.elicitation) = { ... };
  };
}
```

**CounterService** (`proto/counter/v1/counter_service.proto`) — progress layout:

```protobuf
import "mcp/protobuf/annotations.proto";
import "mcp/protobuf/progress.proto";

service CounterService {
  option (mcp.service) = {
    app: { name: "Counter App" version: "1.0.0" description: "..." }
  };

  rpc Count(CountRequest) returns (stream CountStreamChunk) {
    option (mcp.tool) = {
      description: "Counts from 0 up to the given number. Sends progress updates."
      progress: true
    };
  }
}

message CountStreamChunk {
  oneof payload {
    mcp.MCPProgress progress = 1;
    CountResponse result = 2;
  }
}
```

Clients request progress by including `progressToken` in `params._meta` when calling the tool.

**TodoService** uses:
- **`mcp.service`** — app-level metadata
- **`mcp.tool`** — per-RPC tool name/description overrides
- **`mcp.prompt`** — per-RPC prompt templates with schema-based arguments
- **`mcp.elicitation`** — confirmation dialogs with schema-based forms
- **`mcp.field`** — field descriptions, examples, format for tool inputSchema
- **`mcp.enum`** / **`mcp.enum_value`** — enum-level and per-value descriptions
- **`google.api.resource`** — auto-detected MCP resources from AIP resource annotations

**CounterService** uses `progress: true` on the tool option and the oneof layout above. See [Progress](https://github.com/the-protobuf-project/mcp#progress-server-streaming) in the main README for details.

## Code Generation

Install the plugin and generate code for all languages:

```bash
go install github.com/the-protobuf-project/mcp/plugin/cmd/protoc-gen-mcp@latest

cd examples
buf generate
```

This produces:
- `proto/generated/go/` — Go pb + gRPC + MCP files
- `proto/generated/python/` — Python pb + gRPC + MCP files
- `proto/generated/rust/` — Rust pb + gRPC + MCP files

## Generated MCP Tools

**TodoService** — five CRUD operations:

| Tool Name | Description |
| ------------------------------------ | ----------------------------------------- |
| `todo_service-create_todo_v1` | Creates a new todo item |
| `todo_service-get_todo_v1` | Retrieves a todo by resource name |
| `todo_service-list_todos_v1` | Lists todos with pagination |
| `todo_service-update_todo_v1` | Updates an existing todo |
| `todo_service-delete_todo_v1` | Deletes a todo by resource name |

**CounterService** — progress streaming:

| Tool Name | Description |
| ------------------------------------ | ----------------------------------------- |
| `counter_service-count_v1` | Counts from 0 to N with progress updates |

## Language Examples

| Language | Directory | Details |
| -------- | ----------------------- | ---------------------------------- |
| Go | [`go/`](go/) | TodoService (http, stdio, sse, grpc-gateway) + CounterService (counter) |
| Python | [`python/`](python/) | TodoService — `FastMCP` / low-level `Server` |
| Rust | [`rust/`](rust/) | TodoService — `rmcp` SDK with `ServerHandler` |

Each has its own README with setup and run instructions.

## Testing with MCP Inspector

For `streamable-http` or `sse` servers:

```bash
npx @modelcontextprotocol/inspector
# Enter the server URL, e.g.:
#   TodoService:    http://localhost:8082/todo/v1/todoservice/mcp
#   CounterService: http://localhost:8083/counter/v1/counterservice/mcp
```

For `stdio` servers:

```bash
npx @modelcontextprotocol/inspector -- /absolute/path/to/binary
```
