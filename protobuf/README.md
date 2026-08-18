# mcp

[![BSR](https://img.shields.io/badge/BSR-buf.build%2Fthe-protobuf-project%2Fmcp-blue)](https://buf.build/the-protobuf-project/mcp)

Protobuf annotations for exposing gRPC services as [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers.

## Installation

Add this module as a dependency in your `buf.yaml`:

```yaml
version: v2
deps:
  - buf.build/the-protobuf-project/mcp
```

Then run:

```bash
buf dep update
```

## Usage

Import the annotations in your `.proto` files:

```protobuf
import "mcp/v1/annotations.proto";
```

### Service-level options

Configure MCP app metadata on your gRPC service:

```protobuf
service MyService {
  option (mcp.v1.service) = {
    app: {
      display_name: "My App"
      version: "1.0.0"
      description: "A brief description of your application"
    }
  };
}
```

### Tool options

Override the auto-generated MCP tool name or description on individual RPCs:

```protobuf
rpc CreateItem(CreateItemRequest) returns (Item) {
  option (mcp.v1.tool) = {
    description: "Creates a new item with the given fields."
  };
}
```

### Prompt options

Attach a prompt template to an RPC. The `schema` field references a proto message
whose fields define the prompt arguments:

```protobuf
rpc GetItem(GetItemRequest) returns (Item) {
  option (mcp.v1.prompt) = {
    id: "summarize_items"
    description: "Summarize all items for a user"
    schema: "mypackage.SummarizeItemsArgs"
  };
}
```

### Elicitation options

Collect user input before an RPC executes. In **form mode**, `schema` is the
fully-qualified name of the proto message whose fields define the form:

```protobuf
rpc DeleteItem(DeleteItemRequest) returns (google.protobuf.Empty) {
  option (mcp.v1.elicitation) = {
    message: "Are you sure you want to delete this item?"
    schema: "mypackage.DeleteConfirmation"
    required: true
  };
}
```

In **URL mode**, the user is directed to an external URL instead:

```protobuf
rpc Authenticate(AuthRequest) returns (AuthResponse) {
  option (mcp.v1.elicitation) = {
    mode: MCP_ELICITATION_MODE_URL
    message: "Complete authentication via the link below."
    url: "https://example.com/auth"
    elicitation_id: "auth-session-abc"
  };
}
```

`mode` is inferred when left unset — form if `schema` is set, URL if `url` is.
Set `required: true` to abort the tool call when the user declines or the client
does not support elicitation; the default is to proceed anyway.

### Field options

Add JSON Schema metadata to message fields for the MCP tool inputSchema:

```protobuf
string name = 1 [(mcp.v1.field) = {
  description: "Resource name of the item."
  examples: "items/123"
  format: MCP_FIELD_FORMAT_URI
}];
```

`format` is an `MCPFieldFormat` enum (`DATE_TIME`, `DATE`, `TIME`, `URI`, `UUID`,
`EMAIL`, `BYTE`), and `type` accepts an `MCPFieldType` to override the JSON Schema
type inferred from the proto field.

### Enum options

Add descriptions to enum types and individual enum values:

```protobuf
enum Priority {
  option (mcp.v1.enum) = { description: "Priority level." };
  LOW = 0 [(mcp.v1.enum_value) = { description: "Low priority." }];
  HIGH = 1 [(mcp.v1.enum_value) = { description: "High priority." }];
}
```

### Resources

There are two ways to expose MCP resources.

**1. Declare them on the service** with `mcp.v1.service.resources`:

```protobuf
service TodoService {
  option (mcp.v1.service) = {
    resources: [
      {
        pattern: "todo://users/{user}/todos/{todo}"
        id: "Todo"
        title: "Todo item"
        description: "A single todo item belonging to a user."
        mime_type: "application/json"
        size: 2048
        annotations: {
          audience: [MCP_ROLE_USER, MCP_ROLE_ASSISTANT]
          priority: 0.8
        }
        icons: [
          { src: "https://example.com/todo.svg", sizes: ["any"], theme: MCP_ICON_THEME_LIGHT }
        ]
      }
    ]
  };
}
```

`uri` and `pattern` form a oneof — set `uri` for a concrete resource at a fixed
location, or `pattern` for a URI template matching a family of resources. `title`
is the display name preferred over `name`, `annotations` carries audience and
priority hints for the LLM, and `icons` supplies client UI artwork.

**2. Let them be inferred** from the standard
[`google.api.resource`](https://google.aip.dev/123) annotation on the message a
unary RPC **returns**. The generator scans each service's response types and
registers one MCP resource per pattern — no MCP-specific annotation needed:

```protobuf
import "google/api/resource.proto";

message Todo {
  option (google.api.resource) = {
    type: "the-protobuf-project.app.todo.v1/Todo"
    pattern: "users/{user}/todos/{todo}"
    singular: "todo"
    plural: "todos"
  };

  string name = 1;
  string title = 2;
}

service TodoService {
  rpc GetTodo(GetTodoRequest) returns (Todo);
}
```

Each `pattern` becomes a resource template URI, built as `{singular}://{pattern}`:

```text
todo://users/{user}/todos/{todo}
```

which the generator emits as a registered resource template — `AddResourceTemplate`
in Go and `resource_templates` in Rust.

How the fields map:

| `google.api.resource` field | Effect on the MCP resource                                    |
| --------------------------- | ------------------------------------------------------------- |
| `pattern`                   | URI path; one resource is registered per pattern               |
| `singular`                  | URI scheme (falls back to the lowercased kind after `type`'s `/`) |
| `type`                      | Display name, taken from the kind after the `/`                |

The MIME type defaults to `application/json`, patterns are de-duplicated across
methods, and streaming RPCs are skipped. A pattern with no `{placeholder}` is
registered as a fixed resource rather than a template.

Both forms produce the same registration, so use `google.api.resource` when your
protos already follow AIP-123 resource naming, and `mcp.v1.service.resources` when
you need MCP-specific metadata such as `title`, `annotations`, or `icons`.

### Media types

`MCPResource.mime_type` is the single field that tells an MCP client how to
present a resource. The same bytes are prose, markup, a table, an image, or a
download depending only on what this field says:

```protobuf
resources: [
  {uri: "gallery://docs/overview.md"   id: "overview"  mime_type: "text/markdown"},
  {uri: "gallery://docs/report.html"   id: "report"    mime_type: "text/html"},
  {uri: "gallery://data/manifest.json" id: "manifest"  mime_type: "application/json"},
  {uri: "gallery://data/downloads.csv" id: "downloads" mime_type: "text/csv"},
  {uri: "gallery://images/logo.png"    id: "logo"      mime_type: "image/png"},
  {uri: "gallery://docs/spec.pdf"      id: "spec"      mime_type: "application/pdf"}
]
```

**It is a free-form string, not an enum.** The IANA media type registry is
open-ended and gains entries without this schema changing, so constraining it to
a closed set would mean a breaking release every time a consumer needed a type we
had not thought of. This follows [AIP-143](https://google.aip.dev/143), which
reserves enums for closed sets and requires open registries to travel as strings.
Any registered type works, including ones nothing here enumerates — `text/csv`
above is exactly that case.

The value also decides how content travels back from a read. Textual types are
returned as text; binary types are returned as bytes and base64-encoded on the
wire:

| Media type | Returned as | Client typically renders |
| --- | --- | --- |
| `text/markdown` | text | prose |
| `text/html` | text | a document (and the shape an MCP App UI takes) |
| `application/json` | text | structured data the model can parse |
| `text/csv` | text | a table |
| `image/png` | base64 bytes | an inline image |
| `application/pdf` | base64 bytes | a download |

`MCPIcon.mime_type` is the same kind of value, narrowed by convention to image
types — `image/svg+xml`, `image/png`, `image/webp`.

A runnable example that serves one asset under each of these types, together
with the MCP App UI resource, lives in [`examples/mime`](../examples/mime).

> **Note:** `mcp/v1/mime_type.proto` still defines an `MCPMimeType` enum, kept
> from an earlier revision where `mime_type` was enum-typed. Nothing references
> it. Prefer the string; the enum is retained only because removing it from the
> published module is a separate breaking change.

### Progress (server streaming)

For long-running RPCs, use server streaming with `MCPProgress` to report progress:

```protobuf
import "mcp/v1/progress.proto";

message CreateItemStreamChunk {
  oneof payload {
    mcp.v1.MCPProgress progress = 1;
    Item result = 2;
  }
}

rpc CreateItem(CreateItemRequest) returns (stream CreateItemStreamChunk);
```

## Available Protos

| File                            | Description                                                |
| ------------------------------- | ---------------------------------------------------------- |
| `mcp/v1/annotations.proto`      | Service, tool, prompt, elicitation, field, enum extensions |
| `mcp/v1/app.proto`              | `MCPApp` message (name, version, description)              |
| `mcp/v1/prompt.proto`           | `MCPPrompt`, `MCPToolOptions`, `MCPRole`                   |
| `mcp/v1/elicitation.proto`      | `MCPElicitation`, `MCPElicitationMode`                     |
| `mcp/v1/service_options.proto`  | `MCPServiceOptions` (app + resources)                      |
| `mcp/v1/resource.proto`         | `MCPResource`, `MCPAnnotations`, `MCPIcon`, `MCPIconTheme` |
| `mcp/v1/field.proto`            | `MCPFieldOptions`, `MCPFieldFormat`                        |
| `mcp/v1/enum.proto`             | `MCPEnumOptions`, `MCPEnumValueOptions`                    |
| `mcp/v1/progress.proto`         | `MCPProgress` for server-streaming progress                |
| `mcp/v1/field_type.proto`       | `MCPFieldType` enum                                        |
| `mcp/v1/mime_type.proto`        | `MCPMimeType` enum (unreferenced — see note below)          |

> **Note:** `MCPMimeType` is no longer referenced by any message. `MCPResource.mime_type`
> is a plain `string` so that resources can carry any IANA media type, per
> [AIP-143](https://google.aip.dev/143). The enum is retained for now because removing
> it from the published module is a separate breaking change.

## License

Copyright 2026 The Protobuf Project.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use
these files except in compliance with the License. You may obtain a copy of the
License at [apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0),
or in the [`LICENSE`](https://github.com/the-protobuf-project/grpc-mcp-gateway/blob/main/LICENSE)
file at the repository root.

Unless required by applicable law or agreed to in writing, software distributed
under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the
specific language governing permissions and limitations under the License.
