// examples/mime is deliberately its own module, outside go.work and outside the
// examples module.
//
// Its generated code blank-imports github.com/the-protobuf-project/mcp/protobuf/mcppb
// — the mcp.v1 annotations from this repository — while the other examples use
// the published mcp.protobuf ones. Both register extension 51000 on
// google.protobuf.ServiceOptions, and a binary containing the two panics during
// package init:
//
//	proto: extension number 51000 is already registered on message
//	google.protobuf.ServiceOptions
//
// The module boundary is what keeps them apart. runtime-go itself links neither
// (it takes MCPProgress as an interface), so this module builds, tests, and
// serves today. It folds back into examples/ once mcp.v1 is published and every
// example moves to it together; see README.md.
module github.com/the-protobuf-project/mcp/examples/mime

go 1.26.4

require (
	github.com/the-protobuf-project/mcp v0.0.0
	github.com/the-protobuf-project/runtime-go/agents v0.0.0-20260818025400-e63524c03160
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/modelcontextprotocol/go-sdk v1.7.0 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260810153831-ec0a7760b754 // indirect
)

replace github.com/the-protobuf-project/mcp => ../..
