// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// mcpProgressFQNs lists the current McpProgress name and its earlier spellings
// and packages. Keep the old names so protos generated against published
// versions of buf.build/the-protobuf-project/mcp keep working.
var mcpProgressFQNs = map[string]bool{
	"mcp.v1.McpProgress":       true,
	"mcp.v1.MCPProgress":       true,
	"mcp.MCPProgress":          true,
	"mcp.protobuf.MCPProgress": true,
}

// StreamProgressInfo describes a server-streaming RPC that uses McpProgress for progress updates.
// When non-nil, the streamed message has a oneof with McpProgress and a result field.
type StreamProgressInfo struct {
	StreamChunkType  string            // Go type for the streamed message
	StreamClientType string            // Go type for gRPC client stream (e.g. "TodoService_CreateTodoClient")
	StreamServerType string            // Go type for gRPC server stream (e.g. "TodoService_CreateTodoServer")
	ResultType       string            // Go type for the final result (resolved)
	ResultMessage    *protogen.Message // Result message (for Rust type resolution)
	ProgressField    string            // oneof field name for progress (e.g. "Progress")
	ResultField      string            // oneof field name for result (e.g. "Result")
	ServiceName      string            // e.g. "TodoService"
	MethodName       string            // e.g. "CreateTodo"
}

// DetectProgressStream returns StreamProgressInfo if the method is server-streaming
// and the streamed message follows the progress convention: a oneof with
// mcp.v1.McpProgress and the result type.
func DetectProgressStream(meth *protogen.Method, resolveType func(protogen.GoIdent) string) *StreamProgressInfo {
	if !meth.Desc.IsStreamingServer() || meth.Desc.IsStreamingClient() {
		return nil
	}
	msg := meth.Output
	if msg == nil {
		return nil
	}
	// Find the oneof that contains both McpProgress and a result (same oneof).
	var progressField, resultField *protogen.Field
	var resultIdent protogen.GoIdent
	for _, oo := range msg.Oneofs {
		if oo.Desc.IsSynthetic() {
			continue
		}
		var prog, res *protogen.Field
		var resIdent protogen.GoIdent
		for _, f := range oo.Fields {
			if f.Desc.Kind() != protoreflect.MessageKind {
				continue
			}
			fqn := string(f.Message.Desc.FullName())
			if mcpProgressFQNs[fqn] {
				prog = f
			} else {
				res = f
				resIdent = f.Message.GoIdent
			}
		}
		if prog != nil && res != nil {
			progressField = prog
			resultField = res
			resultIdent = resIdent
			break
		}
	}
	if progressField == nil || resultField == nil {
		return nil
	}
	svcName := string(meth.Parent.Desc.Name())
	return &StreamProgressInfo{
		StreamChunkType:  resolveType(msg.GoIdent),
		StreamClientType: svcName + "_" + meth.GoName + "Client",
		StreamServerType: svcName + "_" + meth.GoName + "Server",
		ResultType:       resolveType(resultIdent),
		ResultMessage:    resultField.Message,
		ProgressField:    progressField.GoName,
		ResultField:      resultField.GoName,
		ServiceName:      svcName,
		MethodName:       meth.GoName,
	}
}
