# runtime

Go runtime for [grpc-mcp-gateway](https://github.com/the-protobuf-project/grpc-mcp-gateway) — server configuration, transport handling, and MCP integration helpers.

## Install

```bash
go get github.com/the-protobuf-project/grpc-mcp-gateway/runtime
```

## Overview

The runtime package provides:

- **MCPServerConfig** — Configuration for MCP server name, version, transports, address, and base path
- **StartServer** — Start an MCP server with stdio, streamable-http, or SSE transports
- **Transport** — `stdio`, `streamable-http`, or `sse`
- **HandleError** — Convert gRPC errors to MCP tool error results
- **Schema helpers** — `MustParseSchema`, `MustCreateTool`, `PrepareToolWithExtras`
- **Elicitation** — `RunElicitation`, `ElicitField` for confirmation dialogs
- **Metadata** — `ForwardMetadata`, `HeadersMiddleware`, `DefaultHeaderMappings` for HTTP→gRPC header forwarding
- **App/Resource** — `DefaultPromptHandler`, `DefaultResourceHandler`, `DefaultAppResourceHandler`, `AppResourceURI`, `SetToolAppMeta`
- **Cancellation** — Generated streaming tool handlers honor [MCP cancellation](https://modelcontextprotocol.io/specification/2025-03-26/basic/utilities/cancellation): when the client sends `notifications/cancelled`, the SDK cancels the request context; the gRPC stream returns `context.Canceled`; the handler returns without sending a response

## Quick Start

Generated `Serve*MCP` functions accept your gRPC server implementation and config:

```go
package main

import (
    "context"
    "log"

    "github.com/the-protobuf-project/grpc-mcp-gateway/runtime"
    "github.com/your/module/yourpb"
)

func main() {
    srv := newYourServer()  // implements YourServiceMCPServer (gRPC methods)

    cfg := &runtime.MCPServerConfig{
        Name:                 "my-service",
        Version:              "1.0.0",
        Transports:           []runtime.Transport{runtime.TransportStreamableHTTP},
        Addr:                 ":8082",
        GeneratedBasePath:     yourpb.YourServiceMCPDefaultBasePath,
    }

    err := yourpb.ServeYourServiceMCP(context.Background(), srv, cfg)
    if err != nil {
        log.Fatal(err)
    }
}
```

## Transports

| Transport           | Value             | Use case                          |
| ------------------- | ----------------- | --------------------------------- |
| `TransportStdio`    | `stdio`           | Local tools, IDE integrations     |
| `TransportStreamableHTTP` | `streamable-http` | Production, modern MCP clients    |
| `TransportSSE`      | `sse`             | Legacy SSE clients                |

Multiple transports run concurrently. Parse from env:

```go
transports := runtime.ParseTransports(os.Getenv("MCP_TRANSPORT"))
// e.g. MCP_TRANSPORT=stdio,streamable-http
```

## Configuration

### MCPServerConfig

| Field                 | Description                                      |
| --------------------- | ------------------------------------------------ |
| `Name`                | MCP server name (reported to clients)            |
| `Version`             | Server version                                   |
| `Transport` / `Transports` | Single or multiple transports                 |
| `Addr`                | Listen address for HTTP (default `:8080`)        |
| `BasePath`            | HTTP path prefix (default `/mcp`)                |
| `GeneratedBasePath`   | Proto-derived path (takes precedence)           |
| `HeaderMappings`      | HTTP header → gRPC metadata forwarding          |
| `ReadTimeout`         | Max duration for reading request (0 = no limit)  |
| `WriteTimeout`        | Max duration for writing response (0 = no limit; keep 0 for progress) |
| `OnReady`             | Callback before server starts                    |

### Header forwarding

Forward HTTP headers to gRPC metadata:

```go
cfg := &runtime.MCPServerConfig{
    HeaderMappings: runtime.DefaultHeaderMappings(),
    // Or custom: []runtime.HeaderMapping{
    //     {HTTPHeader: "Authorization", GRPCKey: "authorization"},
    // },
}
```

## Error handling

Convert gRPC errors to MCP tool results:

```go
result, err := myToolHandler(ctx, req)
if err != nil {
    return runtime.HandleError(err)  // Returns (*mcp.CallToolResult, error)
}
```

## Elicitation

Run confirmation dialogs before tool execution. Elicitation is a multi
round-trip request ([SEP-2322]): protocol version `2026-07-28` forbids the
server from asking the client anything while it is serving a request, so the
first pass returns a pending result carrying the question and the client
retries the tool call with the answer. The handler body therefore runs twice.

```go
fields := []runtime.ElicitField{
    {Name: "confirm", Description: "Confirm action", Required: true, Type: "string", EnumValues: []string{"yes", "no"}},
}
result, pending, err := runtime.RunElicitation(req, "Are you sure?", fields)
if err != nil {
    return nil, err
}
if pending != nil {
    return pending, nil // ask the client; it will retry this call with the answer
}
if result.Action != "accept" {
    return runtime.TextResult("Action cancelled by user."), nil
}
```

Clients that negotiated an older protocol version still work unchanged — the
go-sdk's server middleware performs the round trip on their behalf and
reinvokes the handler.

[SEP-2322]: https://modelcontextprotocol.io/seps/2322-multi-round-trip-requests

## Links

- **Source**: [github.com/the-protobuf-project/grpc-mcp-gateway](https://github.com/the-protobuf-project/grpc-mcp-gateway)
- **Examples**: [examples/go](https://github.com/the-protobuf-project/grpc-mcp-gateway/tree/main/examples/go)
- **License**: Apache-2.0
