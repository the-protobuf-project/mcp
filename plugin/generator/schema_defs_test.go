// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// deref is s, or the definition its $ref names under root's $defs.
func deref(root, s map[string]any) map[string]any {
	ref, ok := s["$ref"].(string)
	if !ok {
		return s
	}
	defs, _ := root["$defs"].(map[string]any)
	def, _ := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	return def
}

// A message held by one field stays inline: a small schema reads as before.
func TestMessageSchemaInlinesSingleUse(t *testing.T) {
	root := buildMessage(t,
		&descriptorpb.DescriptorProto{
			Name:  proto.String("Root"),
			Field: []*descriptorpb.FieldDescriptorProto{msgField("only", 1, ".recursion.v1.Leaf", false)},
		},
		&descriptorpb.DescriptorProto{Name: proto.String("Leaf")},
	)
	schema := messageSchema(root, false, "")
	if _, ok := schema["$defs"]; ok {
		t.Errorf("a single-use message went to $defs: %v", schema["$defs"])
	}
	only := schema["properties"].(map[string]any)["only"].(map[string]any)
	if only["type"] != "object" {
		t.Errorf("single-use message not inline: %v", only)
	}
}

// A type graph where every level holds the next twice inlines to 2^depth
// copies of the deepest message; with $defs it is one definition a level.
func TestMessageSchemaSizeFollowsTheGraph(t *testing.T) {
	const depth = 24
	msgs := make([]*descriptorpb.DescriptorProto, depth+1)
	for i := range depth {
		next := fmt.Sprintf(".recursion.v1.L%d", i+1)
		msgs[i] = &descriptorpb.DescriptorProto{
			Name:  proto.String(fmt.Sprintf("L%d", i)),
			Field: []*descriptorpb.FieldDescriptorProto{msgField("a", 1, next, false), msgField("b", 2, next, true)},
		}
	}
	msgs[depth] = &descriptorpb.DescriptorProto{Name: proto.String(fmt.Sprintf("L%d", depth))}
	schema := messageSchema(buildMessage(t, msgs[0], msgs[1:]...), false, "")

	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 16<<10 {
		t.Errorf("schema is %d bytes for a %d-level graph; it grows with the paths, not the types", len(raw), depth)
	}
	if defs := schema["$defs"].(map[string]any); len(defs) != depth {
		t.Errorf("%d definitions; want one for each of the %d shared levels", len(defs), depth)
	}
}
