module github.com/the-protobuf-project/mcp/examples

go 1.26.4

require (
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/the-protobuf-project/mcp v0.0.0-00010101000000-000000000000
	github.com/the-protobuf-project/runtime-go/mcpruntime v0.0.0-00010101000000-000000000000
	google.golang.org/genproto/googleapis/api v0.0.0-20260630182238-925bb5da69e7
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260622175928-b703f567277d // indirect
)

replace github.com/the-protobuf-project/mcp => ../

// The generated MCP runtime moved to runtime-go and has no tagged release yet,
// so it resolves from a sibling checkout. Drop this once it is published.
replace github.com/the-protobuf-project/runtime-go/mcpruntime => ../../runtime/runtime-go/mcpruntime
