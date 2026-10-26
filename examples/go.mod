module github.com/the-protobuf-project/mcp/examples

go 1.26.4

require (
	github.com/modelcontextprotocol/go-sdk v1.8.0
	github.com/the-protobuf-project/runtime-go/agents v0.0.0-20261026204609-216a2112f43e
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require google.golang.org/genproto/googleapis/api v0.0.0-20261019180522-ce908d29299b

require (
	buf.build/gen/go/the-protobuf-project/mcp/protocolbuffers/go v1.36.12-20260622085421-d8286d36a0cc.2
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.2.1 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261019180522-ce908d29299b // indirect
)

// Resolve the plugin and annotations from this checkout rather than a release,
// so the examples exercise the code under test. Workspace members share their
// replace directives, so this path must stay inside the repository -- see go.work.
replace github.com/the-protobuf-project/mcp => ../
