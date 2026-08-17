// Package protobuf provides MCP protobuf annotations and extensions.
//
// Import this package for side effects to register proto extensions on
// google.protobuf.ServiceOptions and MethodOptions. This is required for
// generated code that uses MCP options (mcp.protobuf.service, tool, prompt,
// elicitation).
//
// Usage:
//
//	import _ "github.com/the-protobuf-project/mcp/protobuf"
//
// The blank import is typically added automatically by protoc-gen-go when
// your .proto files import mcp/protobuf/annotations.proto.
//
// For direct type access, use the mcppb subpackage:
//
//	import "github.com/the-protobuf-project/mcp/protobuf/mcppb"
package protobuf

import _ "github.com/the-protobuf-project/mcp/protobuf/mcppb"
