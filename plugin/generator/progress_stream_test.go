// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	mcppb "github.com/the-protobuf-project/mcp/protobuf/mcppb"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestDetectProgressStreamCurrentAndLegacyNames(t *testing.T) {
	for _, tt := range []struct {
		pkg  string
		name string
		want bool
	}{
		{"mcp.v1", "McpProgress", true},
		{"mcp.v1", "MCPProgress", true},
		{"mcp", "MCPProgress", true},
		{"mcp.protobuf", "MCPProgress", true},
		{"other.v1", "McpProgress", false},
	} {
		t.Run(tt.pkg+"."+tt.name, func(t *testing.T) {
			progressFile := &descriptorpb.FileDescriptorProto{
				Name:    proto.String("legacy/progress.proto"),
				Package: proto.String(tt.pkg),
				Syntax:  proto.String("proto3"),
				Options: &descriptorpb.FileOptions{GoPackage: proto.String("example.com/progress;progresspb")},
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: proto.String(tt.name),
				}},
			}
			req := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{"stream.proto"}}
			if tt.pkg == "mcp.v1" && tt.name == "McpProgress" {
				// Use the real schema so another rename cannot silently stop the
				// generator from emitting handlers for current progress streams.
				fd := mcppb.File_mcp_v1_progress_proto
				for i := 0; i < fd.Imports().Len(); i++ {
					req.ProtoFile = append(req.ProtoFile, protodesc.ToFileDescriptorProto(fd.Imports().Get(i)))
				}
				progressFile = protodesc.ToFileDescriptorProto(fd)
			}
			req.ProtoFile = append(req.ProtoFile, progressFile, &descriptorpb.FileDescriptorProto{
				Name:       proto.String("stream.proto"),
				Package:    proto.String("stream.v1"),
				Syntax:     proto.String("proto3"),
				Dependency: []string{progressFile.GetName()},
				Options:    &descriptorpb.FileOptions{GoPackage: proto.String("example.com/stream;streampb")},
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: proto.String("Request")},
					{Name: proto.String("Result")},
					{
						Name:      proto.String("Chunk"),
						OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("payload")}},
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name: proto.String("result"), Number: proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String(".stream.v1.Result"), OneofIndex: proto.Int32(0),
							},
							{
								Name: proto.String("progress"), Number: proto.Int32(2),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: proto.String("." + tt.pkg + "." + tt.name), OneofIndex: proto.Int32(0),
							},
						},
					},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{{
					Name: proto.String("CounterService"),
					Method: []*descriptorpb.MethodDescriptorProto{{
						Name: proto.String("Count"), InputType: proto.String(".stream.v1.Request"),
						OutputType: proto.String(".stream.v1.Chunk"), ServerStreaming: proto.Bool(true),
					}},
				}},
			})
			plugin, err := (protogen.Options{}).New(req)
			if err != nil {
				t.Fatalf("build plugin: %v", err)
			}
			method := plugin.FilesByPath["stream.proto"].Services[0].Methods[0]
			got := DetectProgressStream(method, func(id protogen.GoIdent) string { return id.GoName })
			if !tt.want {
				if got != nil {
					t.Fatal("unrelated progress message was accepted as an MCP progress stream")
				}
				return
			}
			if got == nil {
				t.Fatal("MCP progress stream was not detected")
			}
			if got.ProgressField != "Progress" || got.ResultField != "Result" || got.ResultType != "Result" {
				t.Errorf("wrong stream fields: %+v", got)
			}
		})
	}
}
