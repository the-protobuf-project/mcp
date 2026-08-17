// Package mcppb contains the generated Protocol Buffer types for the mcp.protobuf
// package — MCP annotations used to configure gRPC services for the Model Context Protocol.
//
// # Install
//
//	go get github.com/the-protobuf-project/mcp/protobuf/mcppb
//
// # Types
//
//   - MCPServiceOptions: Service-level app metadata (option mcp.protobuf.service)
//   - MCPToolOptions: Tool name/description overrides (option mcp.protobuf.tool)
//   - MCPPrompt: Prompt template (option mcp.protobuf.prompt)
//   - MCPElicitation: Confirmation dialog (option mcp.protobuf.elicitation)
//   - MCPApp: App name, version, description
//   - MCPResource, MCPMimeType, MCPFieldType: Resource and field types
//
// # Extension Variables
//
// Use with proto.GetExtension to read options from descriptors:
//
//	import "google.golang.org/protobuf/proto"
//
//	opts := proto.GetExtension(serviceDesc.Options(), mcppb.E_Service).(*mcppb.MCPServiceOptions)
//	toolOpts := proto.GetExtension(methodDesc.Options(), mcppb.E_Tool).(*mcppb.MCPToolOptions)
//
// # Proto Registration
//
// For generated code that uses these options, import the parent package for side effects:
//
//	import _ "github.com/the-protobuf-project/mcp/protobuf"
//
// That registers the extension fields so proto.GetExtension works.
package mcppb
