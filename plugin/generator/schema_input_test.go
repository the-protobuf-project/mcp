// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// An input schema describes what a caller sends: fields the server sets are
// left out, and each field carries its proto comment so a model reading the
// schema knows what to put there. An output schema keeps every field.
func TestInputSchemaDropsOutputOnlyAndKeepsComments(t *testing.T) {
	outputOnly := &descriptorpb.FieldOptions{}
	proto.SetExtension(outputOnly, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY})
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("input_test.proto"),
		Package: proto.String("input.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Hold"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("window"), Number: proto.Int32(1), Label: opt, Type: str},
				{Name: proto.String("state"), Number: proto.Int32(2), Label: opt, Type: str, Options: outputOnly},
			},
		}},
		SourceCodeInfo: &descriptorpb.SourceCodeInfo{Location: []*descriptorpb.SourceCodeInfo_Location{{
			Path:            []int32{4, 0, 2, 0}, // message 0, field 0
			Span:            []int32{0, 0, 1},
			LeadingComments: proto.String(" The time to hold, RFC 3339.\n"),
		}}},
	}
	file, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("build file: %v", err)
	}
	md := file.Messages().Get(0)

	in := inputSchema(md, false, "")["properties"].(map[string]any)
	if _, ok := in["state"]; ok {
		t.Errorf("input schema offers the OUTPUT_ONLY field state: %v", in)
	}
	window, _ := in["window"].(map[string]any)
	if window["description"] != "The time to hold, RFC 3339." {
		t.Errorf("window description = %q, want its leading comment", window["description"])
	}
	out := messageSchema(md, false, "")["properties"].(map[string]any)
	if _, ok := out["state"]; !ok {
		t.Errorf("output schema lost state: %v", out)
	}
}
