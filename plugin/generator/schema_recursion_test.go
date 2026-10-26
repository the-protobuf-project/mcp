// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// buildMessage compiles a single-message file descriptor and returns it.
func buildMessage(t *testing.T, msg *descriptorpb.DescriptorProto, extra ...*descriptorpb.DescriptorProto) protoreflect.MessageDescriptor {
	t.Helper()
	fd := &descriptorpb.FileDescriptorProto{
		Name:        proto.String("recursion_test.proto"),
		Package:     proto.String("recursion.v1"),
		Syntax:      proto.String("proto3"),
		MessageType: append([]*descriptorpb.DescriptorProto{msg}, extra...),
	}
	file, err := protodesc.NewFile(fd, nil)
	if err != nil {
		t.Fatalf("build file: %v", err)
	}
	return file.Messages().Get(0)
}

func msgField(name string, num int32, typeName string, repeated bool) *descriptorpb.FieldDescriptorProto {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	return &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		Number:   proto.Int32(num),
		Label:    label.Enum(),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: proto.String(typeName),
	}
}

// A self-referential message is an ordinary shape — every tree and linked list
// has one — and used to recurse until the plugin overflowed its stack.
func TestMessageSchemaHandlesDirectRecursion(t *testing.T) {
	node := buildMessage(t, &descriptorpb.DescriptorProto{
		Name: proto.String("Node"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   proto.String("label"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			},
			msgField("children", 2, ".recursion.v1.Node", true),
		},
	})

	schema := messageSchema(node, false, "")

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties: %v", schema)
	}
	children, ok := props["children"].(map[string]any)
	if !ok {
		t.Fatalf("no children property: %v", props)
	}
	items, ok := children["items"].(map[string]any)
	if !ok {
		t.Fatalf("children has no items: %v", children)
	}
	// The cycle is closed by a reference rather than expanded: Node is
	// written once under $defs, and refers to itself there.
	if items["$ref"] != "#/$defs/recursion.v1.Node" {
		t.Fatalf("recursive field is not a $ref to Node: %v", items)
	}
	def := deref(schema, items)
	inner := def["properties"].(map[string]any)["children"].(map[string]any)["items"].(map[string]any)
	if inner["$ref"] != "#/$defs/recursion.v1.Node" {
		t.Errorf("Node's definition does not refer to itself: %v", inner)
	}
}

// Mutual recursion closes the cycle one level further out.
func TestMessageSchemaHandlesMutualRecursion(t *testing.T) {
	a := buildMessage(t,
		&descriptorpb.DescriptorProto{
			Name:  proto.String("A"),
			Field: []*descriptorpb.FieldDescriptorProto{msgField("b", 1, ".recursion.v1.B", false)},
		},
		&descriptorpb.DescriptorProto{
			Name:  proto.String("B"),
			Field: []*descriptorpb.FieldDescriptorProto{msgField("a", 1, ".recursion.v1.A", false)},
		},
	)

	// Completing at all is the assertion: this overflowed the stack before.
	schema := messageSchema(a, false, "")
	if schema["type"] != "object" {
		t.Fatalf("unexpected root schema: %v", schema)
	}
}

// A message reachable twice by different paths is not a cycle, and must still be
// described in full at both -- once, under $defs, that both refer to --
// rather than being suppressed as if it were one.
func TestMessageSchemaExpandsRepeatedNonCyclicUse(t *testing.T) {
	root := buildMessage(t,
		&descriptorpb.DescriptorProto{
			Name: proto.String("Root"),
			Field: []*descriptorpb.FieldDescriptorProto{
				msgField("left", 1, ".recursion.v1.Leaf", false),
				msgField("right", 2, ".recursion.v1.Leaf", false),
			},
		},
		&descriptorpb.DescriptorProto{
			Name: proto.String("Leaf"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("value"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		},
	)

	schema := messageSchema(root, false, "")
	props := schema["properties"].(map[string]any)
	for _, side := range []string{"left", "right"} {
		leaf, ok := props[side].(map[string]any)
		if !ok {
			t.Fatalf("%s missing: %v", side, props)
		}
		sub, ok := deref(schema, leaf)["properties"].(map[string]any)
		if !ok || sub["value"] == nil {
			t.Errorf("%s was not expanded, but it is not part of a cycle: %v", side, leaf)
		}
	}
}
