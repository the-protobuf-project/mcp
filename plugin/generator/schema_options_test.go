// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// withSchemas runs the test under o, restoring the defaults after.
func withSchemas(t *testing.T, o SchemaOptions) {
	t.Helper()
	prev := Schemas
	Schemas = o
	t.Cleanup(func() { Schemas = prev })
}

// amenities is a message holding the same n-value enum in two fields.
func amenities(t *testing.T, n int) protoreflect.MessageDescriptor {
	t.Helper()
	values := []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("AMENITY_UNSPECIFIED"), Number: proto.Int32(0)}}
	for i := 1; i <= n; i++ {
		values = append(values, &descriptorpb.EnumValueDescriptorProto{
			Name: proto.String(fmt.Sprintf("AMENITY_CODE_%03d", i)), Number: proto.Int32(int32(i)),
		})
	}
	enumField := func(name string, num int32) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name: proto.String(name), Number: proto.Int32(num),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
			TypeName: proto.String(".options.v1.Amenity"),
		}
	}
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("options_test.proto"), Package: proto.String("options.v1"), Syntax: proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Amenity"), Value: values}},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Room"), Field: []*descriptorpb.FieldDescriptorProto{enumField("main", 1), enumField("extra", 2)},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return file.Messages().Get(0)
}

// An enum held by several fields is written once, under $defs.
func TestSharedEnumIsDefinedOnce(t *testing.T) {
	schema := messageSchema(amenities(t, 20), false, "")
	props := schema["properties"].(map[string]any)
	for _, f := range []string{"main", "extra"} {
		if ref := props[f].(map[string]any)["$ref"]; ref != "#/$defs/options.v1.Amenity" {
			t.Errorf("%s = %v; want a $ref to the shared enum", f, props[f])
		}
	}
	def := schema["$defs"].(map[string]any)["options.v1.Amenity"].(map[string]any)
	if vals, _ := def["enum"].([]string); len(vals) != 21 {
		t.Errorf("shared enum lists %d values; want 21", len(vals))
	}
}

// Past max_enum_values an enum is a pattern on its prefix, with examples.
func TestLargeEnumIsAPattern(t *testing.T) {
	withSchemas(t, SchemaOptions{MaxEnumValues: 64})
	schema := messageSchema(amenities(t, 300), false, "")
	def := schema["$defs"].(map[string]any)["options.v1.Amenity"].(map[string]any)
	if _, listed := def["enum"]; listed {
		t.Error("a 301-value enum was listed past max_enum_values=64")
	}
	if def["pattern"] != "^AMENITY_[A-Za-z0-9_]+$" {
		t.Errorf("pattern = %v; want the values' common prefix", def["pattern"])
	}
	if ex, _ := def["examples"].([]any); len(ex) != enumExamples || ex[0] != "AMENITY_CODE_001" {
		t.Errorf("examples = %v; want %d, skipping UNSPECIFIED", def["examples"], enumExamples)
	}
	small := messageSchema(amenities(t, 20), false, "")["$defs"].(map[string]any)["options.v1.Amenity"].(map[string]any)
	if _, listed := small["enum"]; !listed {
		t.Error("an enum under max_enum_values was not listed")
	}
}

func TestFirstSentence(t *testing.T) {
	for in, want := range map[string]string{
		"The room. It has a view.":                     "The room.",
		"Codes, e.g. RACK or BAR. Then more.":          "Codes, e.g. RACK or BAR.",
		"Refers to i.e. the guest. More.":              "Refers to i.e. the guest.",
		"One sentence only":                            "One sentence only",
		"Named by J. Smith in 2003. Revised later.":    "Named by J. Smith in 2003.",
		"Amount (taxes incl.). Paid at the desk.":      "Amount (taxes incl.).",
		"OpenTravel 2003A. (See the code list.) Next.": "OpenTravel 2003A.",
	} {
		if got := firstSentence(in); got != want {
			t.Errorf("firstSentence(%q) = %q; want %q", in, got, want)
		}
	}
}

// Descriptions shorten below the request and what it holds, and in $defs.
func TestShortDescriptionsApplyBelowTheRequest(t *testing.T) {
	withSchemas(t, SchemaOptions{ShortDescriptions: true})
	for _, c := range []struct {
		w    walk
		want bool
	}{{walk{depth: 0}, false}, {walk{depth: 1}, false}, {walk{depth: 2}, true}, {walk{depth: 0, inDef: true}, true}} {
		if got := c.w.short(); got != c.want {
			t.Errorf("depth %d, inDef %v: short = %v; want %v", c.w.depth, c.w.inDef, got, c.want)
		}
	}
	withSchemas(t, SchemaOptions{})
	if (&walk{depth: 5, inDef: true}).short() {
		t.Error("descriptions shortened without short_descriptions")
	}
}

func TestNoOutputSchema(t *testing.T) {
	md := amenities(t, 3)
	if outputSchemaFor(md) == "" {
		t.Error("output schema empty by default")
	}
	withSchemas(t, SchemaOptions{NoOutputSchema: true})
	if got := outputSchemaFor(md); got != "" {
		t.Errorf("output_schema=false still wrote %q", got)
	}
}
