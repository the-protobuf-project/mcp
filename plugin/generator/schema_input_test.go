// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"encoding/json"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
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

// An input field whose rules are switched off with IGNORE_ALWAYS -- the
// resource of an AIP-134 partial update -- states nothing its message requires
// or constrains: neither buf.validate keywords nor REQUIRED field_behavior.
// The same message reached through a field that keeps its rules, and every
// output schema, still carry both.
func TestInputSchemaIgnoreAlwaysDropsNestedRules(t *testing.T) {
	required := &descriptorpb.FieldOptions{}
	proto.SetExtension(required, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_REQUIRED})
	ignored := func() *descriptorpb.FieldOptions {
		o := &descriptorpb.FieldOptions{}
		proto.SetExtension(o, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_REQUIRED})
		proto.SetExtension(o, validate.E_Field, validate.FieldRules_builder{Ignore: validate.Ignore_IGNORE_ALWAYS.Enum()}.Build())
		return o
	}
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()

	hold := ruledField("hold", 1, str, validate.FieldRules_builder{
		String: validate.StringRules_builder{Pattern: proto.String("^holds/[^/]+$")}.Build(),
	}.Build())
	proto.SetExtension(hold.Options, annotations.E_FieldBehavior, []annotations.FieldBehavior{annotations.FieldBehavior_REQUIRED})
	tags := repeatedField(ruledField("tags", 3, str, validate.FieldRules_builder{
		Repeated: validate.RepeatedRules_builder{MinItems: proto.Uint64(1)}.Build(),
	}.Build()))
	file := buildFile(t, &descriptorpb.FileDescriptorProto{
		Name:    proto.String("ignore_test.proto"),
		Package: proto.String("ignore.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("UpdateReservationRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("reservation"), Number: proto.Int32(1), Label: opt, Type: msg, TypeName: proto.String(".ignore.v1.Reservation"), Options: ignored()},
					{Name: proto.String("history"), Number: proto.Int32(2), Label: rep, Type: msg, TypeName: proto.String(".ignore.v1.Reservation"), Options: ignored()},
					{Name: proto.String("by_key"), Number: proto.Int32(3), Label: rep, Type: msg, TypeName: proto.String(".ignore.v1.UpdateReservationRequest.ByKeyEntry"), Options: ignored()},
					{Name: proto.String("original"), Number: proto.Int32(4), Label: opt, Type: msg, TypeName: proto.String(".ignore.v1.Reservation"), Options: required},
				},
				NestedType: []*descriptorpb.DescriptorProto{{
					Name: proto.String("ByKeyEntry"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{Name: proto.String("key"), Number: proto.Int32(1), Label: opt, Type: str.Enum()},
						{Name: proto.String("value"), Number: proto.Int32(2), Label: opt, Type: msg, TypeName: proto.String(".ignore.v1.Reservation")},
					},
					Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
				}},
			},
			{
				Name: proto.String("Reservation"),
				Field: []*descriptorpb.FieldDescriptorProto{
					hold,
					{Name: proto.String("slot"), Number: proto.Int32(2), Label: opt, Type: msg, TypeName: proto.String(".ignore.v1.Slot"), Options: required},
					tags,
				},
			},
			{
				Name: proto.String("Slot"),
				Field: []*descriptorpb.FieldDescriptorProto{
					ruledField("id", 1, str, validate.FieldRules_builder{Required: proto.Bool(true)}.Build()),
				},
			},
		},
	})
	md := file.Messages().Get(0)
	asJSON := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(raw)
	}

	in := inputSchema(md, false, "")
	if got, want := asJSON(in["required"]), `["reservation","history","by_key","original"]`; got != want {
		t.Errorf("request required = %s, want %s", got, want)
	}
	props := in["properties"].(map[string]any)
	reservation := props["reservation"].(map[string]any)
	unruled := `{"properties":{"hold":{"type":"string"},"slot":{"properties":{"id":{"type":"string"}},"required":[],"type":"object"},"tags":{"items":{"type":"string"},"type":"array"}},"required":[],"type":"object"}`
	if got := asJSON(reservation); got != unruled {
		t.Errorf("IGNORE_ALWAYS reservation = %s, want %s", got, unruled)
	}
	if got := asJSON(props["history"].(map[string]any)["items"]); got != unruled {
		t.Errorf("IGNORE_ALWAYS repeated items = %s, want %s", got, unruled)
	}
	if got := asJSON(props["by_key"].(map[string]any)["additionalProperties"]); got != unruled {
		t.Errorf("IGNORE_ALWAYS map values = %s, want %s", got, unruled)
	}
	ruled := `{"properties":{"hold":{"pattern":"^holds/[^/]+$","type":"string"},"slot":{"properties":{"id":{"type":"string"}},"required":["id"],"type":"object"},"tags":{"items":{"type":"string"},"minItems":1,"type":"array"}},"required":["hold","slot"],"type":"object"}`
	if got := asJSON(props["original"]); got != ruled {
		t.Errorf("ruled reservation = %s, want %s", got, ruled)
	}
	if got := asJSON(messageSchema(md, false, "")["properties"].(map[string]any)["reservation"]); got != ruled {
		t.Errorf("output reservation = %s, want %s", got, ruled)
	}
	// OpenAI strict mode lists every property as required whatever the rules.
	strict := inputSchema(md, true, "")["properties"].(map[string]any)["reservation"].(map[string]any)
	if got, want := asJSON(strict["required"]), `["hold","slot","tags"]`; got != want {
		t.Errorf("OpenAI reservation required = %s, want %s", got, want)
	}
}
