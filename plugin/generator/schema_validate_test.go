// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"encoding/json"
	"testing"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func ruledField(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, rules *validate.FieldRules) *descriptorpb.FieldDescriptorProto {
	f := &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(name),
		Number: proto.Int32(num),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   typ.Enum(),
	}
	if rules != nil {
		f.Options = &descriptorpb.FieldOptions{}
		proto.SetExtension(f.Options, validate.E_Field, rules)
	}
	return f
}

func repeatedField(f *descriptorpb.FieldDescriptorProto) *descriptorpb.FieldDescriptorProto {
	f.Label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()
	return f
}

func buildFile(t *testing.T, fd *descriptorpb.FileDescriptorProto) protoreflect.FileDescriptor {
	t.Helper()
	file, err := protodesc.NewFile(fd, nil)
	if err != nil {
		t.Fatalf("build file: %v", err)
	}
	return file
}

// schemaJSON builds the schema of a single-message file and returns it as
// canonical JSON, so expectations read like the schema a client receives.
func schemaJSON(t *testing.T, openAI bool, msg *descriptorpb.DescriptorProto, enums ...*descriptorpb.EnumDescriptorProto) map[string]any {
	t.Helper()
	fd := &descriptorpb.FileDescriptorProto{
		Name:        proto.String("validate_test.proto"),
		Package:     proto.String("validate.v1"),
		Syntax:      proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{msg},
		EnumType:    enums,
	}
	md := buildFile(t, fd).Messages().Get(0)
	raw, err := json.Marshal(messageSchema(md, openAI, ""))
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return out
}

func property(t *testing.T, schema map[string]any, name string) string {
	t.Helper()
	props, _ := schema["properties"].(map[string]any)
	raw, err := json.Marshal(props[name])
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	return string(raw)
}

func requiredJSON(t *testing.T, schema map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(schema["required"])
	return string(raw)
}

func anyOfJSON(t *testing.T, schema map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(schema["anyOf"])
	if err != nil {
		t.Fatalf("marshal anyOf: %v", err)
	}
	return string(raw)
}

func TestValidateRequiredMarksFieldsRequired(t *testing.T) {
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	schema := schemaJSON(t, false, &descriptorpb.DescriptorProto{
		Name: proto.String("Req"),
		Field: []*descriptorpb.FieldDescriptorProto{
			ruledField("id", 1, str, validate.FieldRules_builder{Required: proto.Bool(true)}.Build()),
			ruledField("ignored", 2, str, validate.FieldRules_builder{
				Required: proto.Bool(true),
				Ignore:   validate.Ignore_IGNORE_ALWAYS.Enum(),
			}.Build()),
			ruledField("optional", 3, str, nil),
			func() *descriptorpb.FieldDescriptorProto {
				f := ruledField("by_email", 4, str, validate.FieldRules_builder{Required: proto.Bool(true)}.Build())
				f.OneofIndex = proto.Int32(0)
				return f
			}(),
		},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("lookup")}},
	})

	if got, want := requiredJSON(t, schema), `["id"]`; got != want {
		t.Errorf("required = %s, want %s", got, want)
	}
}

func TestMessageSchemaAnyOfFollowsOneofDeclarationOrder(t *testing.T) {
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	member := func(name string, num, oneof int32) *descriptorpb.FieldDescriptorProto {
		f := ruledField(name, num, str, nil)
		f.OneofIndex = proto.Int32(oneof)
		return f
	}
	schema := schemaJSON(t, false, &descriptorpb.DescriptorProto{
		Name: proto.String("Echo"),
		Field: []*descriptorpb.FieldDescriptorProto{
			ruledField("id", 1, str, nil),
			member("zed_a", 2, 0),
			member("zed_b", 3, 0),
			member("alpha_a", 4, 1),
			member("alpha_b", 5, 1),
		},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{
			{Name: proto.String("zed")},
			{Name: proto.String("alpha")},
		},
	})
	got := anyOfJSON(t, schema)
	want := `[{"$comment":"Protobuf oneOf group.","oneOf":[{"properties":{"zed_a":{"type":"string"}},"required":["zed_a"]},{"properties":{"zed_b":{"type":"string"}},"required":["zed_b"]}]},{"$comment":"Protobuf oneOf group.","oneOf":[{"properties":{"alpha_a":{"type":"string"}},"required":["alpha_a"]},{"properties":{"alpha_b":{"type":"string"}},"required":["alpha_b"]}]}]`
	if got != want {
		t.Errorf("anyOf = %s, want %s", got, want)
	}
}

func TestValidateRulesBecomeSchemaKeywords(t *testing.T) {
	cases := []struct {
		name   string
		field  *descriptorpb.FieldDescriptorProto
		openAI bool
		want   string
	}{
		{
			name: "uint32 inclusive range",
			field: ruledField("limit", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT32, validate.FieldRules_builder{
				Uint32: validate.UInt32Rules_builder{Gte: proto.Uint32(1), Lte: proto.Uint32(100)}.Build(),
			}.Build()),
			want: `{"maximum":100,"minimum":1,"type":"integer"}`,
		},
		{
			name: "int32 exclusive range steps inwards",
			field: ruledField("n", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, validate.FieldRules_builder{
				Int32: validate.Int32Rules_builder{Gt: proto.Int32(0), Lt: proto.Int32(10)}.Build(),
			}.Build()),
			want: `{"maximum":9,"minimum":1,"type":"integer"}`,
		},
		{
			name: "double exclusive range",
			field: ruledField("budget", 1, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, validate.FieldRules_builder{
				Double: validate.DoubleRules_builder{Gt: proto.Float64(0), Lte: proto.Float64(1.5)}.Build(),
			}.Build()),
			want: `{"exclusiveMinimum":0,"maximum":1.5,"type":"number"}`,
		},
		{
			name: "exclusive outside range is left to the server",
			field: ruledField("n", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, validate.FieldRules_builder{
				Int32: validate.Int32Rules_builder{Gt: proto.Int32(10), Lt: proto.Int32(5)}.Build(),
			}.Build()),
			want: `{"type":"integer"}`,
		},
		{
			name: "int32 const and in",
			field: ruledField("n", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, validate.FieldRules_builder{
				Int32: validate.Int32Rules_builder{In: []int32{1, 2}}.Build(),
			}.Build()),
			want: `{"enum":[1,2],"type":"integer"}`,
		},
		{
			name: "string in, len and uri",
			field: ruledField("s", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				String: validate.StringRules_builder{In: []string{"a", "b"}, Len: proto.Uint64(1), Uri: proto.Bool(true)}.Build(),
			}.Build()),
			want: `{"enum":["a","b"],"format":"uri","maxLength":1,"minLength":1,"type":"string"}`,
		},
		{
			name: "ignore if zero value still accepts the zero value",
			field: ruledField("s", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Ignore: validate.Ignore_IGNORE_IF_ZERO_VALUE.Enum(),
				String: validate.StringRules_builder{MinLen: proto.Uint64(3)}.Build(),
			}.Build()),
			want: `{"anyOf":[{"const":""},{"minLength":3}],"type":"string"}`,
		},
		{
			name: "ignore always drops every rule",
			field: ruledField("s", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Ignore: validate.Ignore_IGNORE_ALWAYS.Enum(),
				String: validate.StringRules_builder{MinLen: proto.Uint64(3)}.Build(),
			}.Build()),
			want: `{"type":"string"}`,
		},
		{
			name: "repeated counts and item rules",
			field: repeatedField(ruledField("tags", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Repeated: validate.RepeatedRules_builder{
					MinItems: proto.Uint64(1),
					MaxItems: proto.Uint64(30),
					Unique:   proto.Bool(true),
					Items: validate.FieldRules_builder{
						String: validate.StringRules_builder{MaxLen: proto.Uint64(8)}.Build(),
					}.Build(),
				}.Build(),
			}.Build())),
			want: `{"items":{"maxLength":8,"type":"string"},"maxItems":30,"minItems":1,"type":"array","uniqueItems":true}`,
		},
		{
			name: "item ignore always drops item rules",
			field: repeatedField(ruledField("tags", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Repeated: validate.RepeatedRules_builder{
					Items: validate.FieldRules_builder{
						Ignore: validate.Ignore_IGNORE_ALWAYS.Enum(),
						String: validate.StringRules_builder{MinLen: proto.Uint64(3)}.Build(),
					}.Build(),
				}.Build(),
			}.Build())),
			want: `{"items":{"type":"string"},"type":"array"}`,
		},
		{
			name: "openai keeps only strict mode keywords",
			field: ruledField("s", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				String: validate.StringRules_builder{
					Const:  proto.String("a"),
					MinLen: proto.Uint64(1),
					Uri:    proto.Bool(true),
				}.Build(),
			}.Build()),
			openAI: true,
			want:   `{"enum":["a"],"type":"string"}`,
		},
		{
			name: "openai drops unique items and keeps item counts",
			field: repeatedField(ruledField("tags", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Repeated: validate.RepeatedRules_builder{
					MaxItems: proto.Uint64(30),
					Unique:   proto.Bool(true),
				}.Build(),
			}.Build())),
			openAI: true,
			want:   `{"items":{"type":"string"},"maxItems":30,"type":"array"}`,
		},
		{
			name: "openai leaves ignore if zero value fields unconstrained",
			field: ruledField("s", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{
				Ignore: validate.Ignore_IGNORE_IF_ZERO_VALUE.Enum(),
				String: validate.StringRules_builder{Pattern: proto.String("^[a-f0-9]+$")}.Build(),
			}.Build()),
			openAI: true,
			want:   `{"type":"string"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schema := schemaJSON(t, c.openAI, &descriptorpb.DescriptorProto{
				Name:  proto.String("Req"),
				Field: []*descriptorpb.FieldDescriptorProto{c.field},
			})
			if got := property(t, schema, c.field.GetName()); got != c.want {
				t.Errorf("schema = %s\nwant      %s", got, c.want)
			}
		})
	}
}

func TestValidateEnumRulesNarrowValues(t *testing.T) {
	status := &descriptorpb.EnumDescriptorProto{
		Name: proto.String("Status"),
		Value: []*descriptorpb.EnumValueDescriptorProto{
			{Name: proto.String("STATUS_UNSPECIFIED"), Number: proto.Int32(0)},
			{Name: proto.String("STATUS_PLAYING"), Number: proto.Int32(1)},
			{Name: proto.String("STATUS_PAUSED"), Number: proto.Int32(2)},
		},
	}
	enumField := func(rules *validate.FieldRules) *descriptorpb.FieldDescriptorProto {
		f := ruledField("status", 1, descriptorpb.FieldDescriptorProto_TYPE_ENUM, rules)
		f.TypeName = proto.String(".validate.v1.Status")
		return f
	}
	cases := []struct {
		name  string
		rules *validate.EnumRules
		want  string
	}{
		{
			name:  "not in",
			rules: validate.EnumRules_builder{NotIn: []int32{0}}.Build(),
			want:  `{"enum":["STATUS_PLAYING","STATUS_PAUSED"],"type":"string"}`,
		},
		{
			name:  "in",
			rules: validate.EnumRules_builder{In: []int32{2}}.Build(),
			want:  `{"enum":["STATUS_PAUSED"],"type":"string"}`,
		},
		{
			name:  "const",
			rules: validate.EnumRules_builder{Const: proto.Int32(1)}.Build(),
			want:  `{"const":"STATUS_PLAYING","enum":["STATUS_UNSPECIFIED","STATUS_PLAYING","STATUS_PAUSED"],"type":"string"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schema := schemaJSON(t, false, &descriptorpb.DescriptorProto{
				Name:  proto.String("Req"),
				Field: []*descriptorpb.FieldDescriptorProto{enumField(validate.FieldRules_builder{Enum: c.rules}.Build())},
			}, status)
			if got := property(t, schema, "status"); got != c.want {
				t.Errorf("schema = %s\nwant      %s", got, c.want)
			}
		})
	}
}

// buf and protoc refuse to run a plugin on an editions file unless the plugin
// declares editions support in its response.
func TestGenerateAcceptsEditionsFiles(t *testing.T) {
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"svc/v1/svc.proto"},
		ProtoFile: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("svc/v1/svc.proto"),
			Package: proto.String("svc.v1"),
			Syntax:  proto.String("editions"),
			Edition: descriptorpb.Edition_EDITION_2023.Enum(),
			MessageType: []*descriptorpb.DescriptorProto{
				{Name: proto.String("Req"), Field: []*descriptorpb.FieldDescriptorProto{
					ruledField("id", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, validate.FieldRules_builder{Required: proto.Bool(true)}.Build()),
				}},
				{Name: proto.String("Resp")},
			},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("Svc"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("Get"),
					InputType:  proto.String(".svc.v1.Req"),
					OutputType: proto.String(".svc.v1.Resp"),
				}},
			}},
			Options: &descriptorpb.FileOptions{GoPackage: proto.String("example.com/svc/v1;svcv1")},
		}},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("build plugin: %v", err)
	}
	if err := Generate(plugin, "go", ""); err != nil {
		t.Fatalf("generate: %v", err)
	}

	resp := plugin.Response()
	if resp.GetError() != "" {
		t.Fatalf("response error: %s", resp.GetError())
	}
	if resp.GetSupportedFeatures()&uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS) == 0 {
		t.Errorf("supported features %b lack SUPPORTS_EDITIONS", resp.GetSupportedFeatures())
	}
	if got := descriptorpb.Edition(resp.GetMaximumEdition()); got < descriptorpb.Edition_EDITION_2023 {
		t.Errorf("maximum edition = %s, want at least EDITION_2023", got)
	}
	if len(resp.GetFile()) == 0 {
		t.Error("no files generated for the editions file")
	}
}
