// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"
	"strings"

	"buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	mcppb "github.com/the-protobuf-project/mcp/protobuf/mcppb"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// kindToType maps a protobuf scalar kind to a JSON Schema type string.
func kindToType(kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.BoolKind:
		return "boolean"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return "integer"
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return "number"
	default: // string, bytes, enum, int64 variants (JSON-encoded as strings)
		return "string"
	}
}

// isFieldRequired reports whether a field is required, either through REQUIRED
// google.api.field_behavior or through (buf.validate.field).required.
func isFieldRequired(fd protoreflect.FieldDescriptor) bool {
	if isValidateRequired(fd) {
		return true
	}
	if !proto.HasExtension(fd.Options(), annotations.E_FieldBehavior) {
		return false
	}
	behaviors := proto.GetExtension(fd.Options(), annotations.E_FieldBehavior).([]annotations.FieldBehavior)
	for _, b := range behaviors {
		if b == annotations.FieldBehavior_REQUIRED {
			return true
		}
	}
	return false
}

// walk is one schema walk's state: the message types open on the current
// branch (see messageSchemaPath), whether the schema describes a tool's
// input -- what a caller sends, which leaves out the fields a server sets --
// whether the walk is beneath an input field whose rules are switched off
// with IGNORE_ALWAYS (see unruledInput), and the messages written once under
// the root's $defs (see schema_defs.go): which ones, and those written so far.
type walk struct {
	open    map[protoreflect.FullName]bool
	input   bool
	unruled bool
	shared  map[defKey]bool
	defs    map[string]any
}

// messageSchema converts a protobuf message descriptor into a JSON Schema map.
// If schemaDesc is non-empty, it is set as the root-level description (per MCP inputSchema convention).
func messageSchema(md protoreflect.MessageDescriptor, openAI bool, schemaDesc string) map[string]any {
	return rootSchema(md, openAI, schemaDesc, false)
}

// inputSchema is messageSchema for a tool's input: OUTPUT_ONLY fields -- a
// name, an etag, a state, the timestamps a server sets -- are left out at any
// depth, since a caller sending them is ignored or refused. A resource name is
// often OUTPUT_ONLY on create yet the identifier elsewhere; it is left out
// only where its own field says OUTPUT_ONLY.
func inputSchema(md protoreflect.MessageDescriptor, openAI bool, schemaDesc string) map[string]any {
	return rootSchema(md, openAI, schemaDesc, true)
}

// rootSchema is md's schema, inline at the root, with the messages it shares
// among its fields under $defs.
func rootSchema(md protoreflect.MessageDescriptor, openAI bool, schemaDesc string, input bool) map[string]any {
	w := &walk{open: map[protoreflect.FullName]bool{}, input: input, shared: sharedDefs(md, input), defs: map[string]any{}}
	schema := messageSchemaPath(md, openAI, schemaDesc, w)
	if len(w.defs) > 0 {
		schema["$defs"] = w.defs
	}
	return schema
}

// sharedRef is the $ref to md as the walk renders it here, writing its
// definition first if this is its first use; ok is false for a message
// rendered inline.
func sharedRef(md protoreflect.MessageDescriptor, openAI bool, w *walk) (map[string]any, bool) {
	k := defKey{name: md.FullName(), unruled: w.unruled}
	if !w.shared[k] {
		return nil, false
	}
	if _, done := w.defs[k.ref()]; !done {
		w.defs[k.ref()] = nil // claimed: a cycle back to md refers rather than recurses
		// A definition renders whole wherever it is first reached: the
		// branch's open set is not its own.
		open := w.open
		w.open = map[protoreflect.FullName]bool{}
		w.defs[k.ref()] = messageSchemaPath(md, openAI, "", w)
		w.open = open
	}
	return map[string]any{"$ref": "#/$defs/" + k.ref()}, true
}

// isOutputOnly reports a field the server sets: google.api.field_behavior
// OUTPUT_ONLY.
func isOutputOnly(fd protoreflect.FieldDescriptor) bool {
	if !proto.HasExtension(fd.Options(), annotations.E_FieldBehavior) {
		return false
	}
	for _, b := range proto.GetExtension(fd.Options(), annotations.E_FieldBehavior).([]annotations.FieldBehavior) {
		if b == annotations.FieldBehavior_OUTPUT_ONLY {
			return true
		}
	}
	return false
}

// fieldComment is fd's leading comment, cleaned, when the descriptor carries
// its source (a generator's always does), for a field without (mcp.field).
func fieldComment(fd protoreflect.FieldDescriptor) string {
	loc := fd.ParentFile().SourceLocations().ByDescriptor(fd)
	return CleanComment(loc.LeadingComments)
}

// messageSchemaPath is messageSchema carrying the set of message types already
// open on the current branch.
//
// A self-referential message — `Node { repeated Node children }`, the shape of
// every tree and linked list — would otherwise recurse until the stack gives
// out. JSON Schema cannot express an infinitely nested object literally, so at
// the point the cycle closes the walk stops and emits a permissive object,
// which is true (the value is an object) without claiming to enumerate it.
func messageSchemaPath(md protoreflect.MessageDescriptor, openAI bool, schemaDesc string, w *walk) map[string]any {
	if w.open[md.FullName()] {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
			"description": fmt.Sprintf(
				"Recursive reference to %s; nested occurrences are not expanded.", md.FullName()),
		}
	}
	w.open[md.FullName()] = true
	defer delete(w.open, md.FullName())

	required, props := []string{}, map[string]any{}
	oneOfGroups := map[string][]map[string]any{}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if w.input && isOutputOnly(fd) {
			continue
		}
		name := string(fd.Name())
		if oo := fd.ContainingOneof(); oo != nil && !oo.IsSynthetic() {
			if !openAI {
				key := string(oo.Name())
				oneOfGroups[key] = append(oneOfGroups[key], map[string]any{
					"properties": map[string]any{name: fieldSchemaPath(fd, openAI, w)}, "required": []string{name},
				})
			} else {
				s := fieldSchemaPath(fd, openAI, w)
				if t, ok := s["type"].(string); ok {
					s["type"] = []string{t, "null"}
				}
				s["description"] = fmt.Sprintf("Note: Part of the '%s' oneof group. Only one field in this group can be set. Setting multiple fields WILL result in an error.", oo.Name())
				props[name] = s
				required = append(required, name)
			}
		} else {
			props[name] = fieldSchemaPath(fd, openAI, w)
			if (isFieldRequired(fd) && !w.unruled) || openAI {
				required = append(required, name)
			}
		}
	}
	result := map[string]any{"type": "object", "properties": props, "required": required}
	if schemaDesc != "" {
		result["description"] = schemaDesc
	}
	if len(oneOfGroups) > 0 {
		var anyOf []map[string]any
		for _, entries := range oneOfGroups {
			anyOf = append(anyOf, map[string]any{"oneOf": entries, "$comment": "Protobuf oneOf group."})
		}
		result["anyOf"] = anyOf
	}
	if openAI {
		result["additionalProperties"] = false
		if t, ok := result["type"].(string); ok {
			result["type"] = []string{t, "null"}
		}
	}
	return result
}

// getFieldDescription returns the field description from (mcp.field) if set,
// otherwise the fallback (e.g. from the leading comment).
func getFieldDescription(fd protoreflect.FieldDescriptor, fallback string) string {
	if proto.HasExtension(fd.Options(), mcppb.E_Field) {
		opts := proto.GetExtension(fd.Options(), mcppb.E_Field).(*mcppb.MCPFieldOptions)
		if opts != nil && opts.Description != "" {
			return opts.Description
		}
	}
	return fallback
}

// applyMCPFieldOptions applies (mcp.field) options to the schema.
// Format from MCPFieldOptions overrides any from buf.validate.
// For enum fields with (mcp.enum) or (mcp.enum_value), enum descriptions take precedence over field description.
func applyMCPFieldOptions(fd protoreflect.FieldDescriptor, schema map[string]any, descFallback string) {
	if !proto.HasExtension(fd.Options(), mcppb.E_Field) {
		if descFallback != "" {
			schema["description"] = descFallback
		}
		return
	}
	opts := proto.GetExtension(fd.Options(), mcppb.E_Field).(*mcppb.MCPFieldOptions)
	if opts == nil {
		return
	}
	// For enum fields, prefer enum-level and enum-value descriptions over field description
	if fd.Kind() == protoreflect.EnumKind && schema["description"] != nil && schema["description"] != "" {
		// Enum descriptions already set by enumSchema; skip field description
	} else if opts.Description != "" {
		schema["description"] = opts.Description
	} else if descFallback != "" {
		schema["description"] = descFallback
	}
	if len(opts.Examples) > 0 {
		examples := make([]any, len(opts.Examples))
		for i, e := range opts.Examples {
			examples[i] = e
		}
		schema["examples"] = examples
	}
	if opts.Deprecated {
		schema["deprecated"] = true
	}
	format := fieldFormatKeyword(opts.GetFormat())
	if format == "" {
		format = legacyFieldFormat(opts)
	}
	if format != "" {
		schema["format"] = format
	}
	// An explicit type overrides the one inferred from the proto field, for
	// fields whose wire type does not match their intended schema type (e.g. an
	// int64 that should appear as "string" in JSON).
	//
	// Inferred numeric bounds survive a numeric-to-numeric override, where they
	// still describe the value; they are dropped only when the value stops being
	// a number, because "minimum" against a string is meaningless.
	if typ := fieldTypeKeyword(opts.GetType()); typ != "" && schema["type"] != typ {
		schema["type"] = typ
		if typ != "integer" && typ != "number" {
			delete(schema, "minimum")
			delete(schema, "maximum")
		}
	}
}

// fieldSchemaPath converts a single protobuf field descriptor to a JSON Schema
// map, carrying the set of message types already open on this branch so a
// recursive field can be cut rather than followed.
func fieldSchemaPath(fd protoreflect.FieldDescriptor, openAI bool, w *walk) map[string]any {
	if !w.unruled && unruledInput(fd, w) {
		w.unruled = true
		defer func() { w.unruled = false }()
	}
	if fd.IsMap() {
		schema := mapSchemaPath(fd, openAI, w)
		if !openAI && !w.unruled {
			applyContainerRules(fd, schema, false)
		}
		return schema
	}
	var schema map[string]any
	switch fd.Kind() {
	case protoreflect.MessageKind:
		schema = messageFieldSchemaPath(fd, openAI, w)
	case protoreflect.EnumKind:
		schema = enumSchema(fd)
	default:
		schema = scalarSchema(fd, openAI)
	}
	if !fd.IsList() {
		if !w.unruled {
			applyValidateRules(fd, schema, openAI)
		}
		applyMCPFieldOptions(fd, schema, fieldComment(fd))
		return schema
	}
	if !w.unruled {
		applyRules(fd, itemRules(fd), schema, openAI)
	}
	applyMCPFieldOptions(fd, schema, fieldComment(fd))
	array := map[string]any{"type": "array", "items": schema}
	if !w.unruled {
		applyContainerRules(fd, array, openAI)
	}
	return array
}

// unruledInput reports an input field of message type -- singular, repeated
// or a map of messages -- carrying (buf.validate.field).ignore = IGNORE_ALWAYS.
//
// protovalidate then checks nothing on the field or inside the message it
// holds, so the message's own contract is not this request's: the canonical
// case is an AIP-134 Update, whose resource carries only the fields the mask
// names and so cannot be held to what a create must send. Beneath such a
// field the schema states no buf.validate keyword and no required field --
// neither (buf.validate.field).required nor google.api.field_behavior
// REQUIRED, which describes the resource, not this partial copy of it. The
// field's own required-ness is unchanged: it is decided by the enclosing
// message.
func unruledInput(fd protoreflect.FieldDescriptor, w *walk) bool {
	if !w.input || !proto.HasExtension(fd.Options(), validate.E_Field) {
		return false
	}
	value := fd
	if fd.IsMap() {
		value = fd.MapValue()
	}
	if value.Kind() != protoreflect.MessageKind {
		return false
	}
	rules, _ := proto.GetExtension(fd.Options(), validate.E_Field).(*validate.FieldRules)
	return rules.GetIgnore() == validate.Ignore_IGNORE_ALWAYS
}

// enumDescriptions holds enum-level and per-value descriptions for schema output.
type enumDescriptions struct {
	enumDesc string
	values   map[string]string // value name -> description
}

func getEnumDescriptions(ed protoreflect.EnumDescriptor) enumDescriptions {
	out := enumDescriptions{values: make(map[string]string)}
	if proto.HasExtension(ed.Options(), mcppb.E_Enum) {
		opts := proto.GetExtension(ed.Options(), mcppb.E_Enum).(*mcppb.MCPEnumOptions)
		if opts != nil && opts.Description != "" {
			out.enumDesc = opts.Description
		}
	}
	vals := ed.Values()
	for i := 0; i < vals.Len(); i++ {
		vd := vals.Get(i)
		if proto.HasExtension(vd.Options(), mcppb.E_EnumValue) {
			opts := proto.GetExtension(vd.Options(), mcppb.E_EnumValue).(*mcppb.MCPEnumValueOptions)
			if opts != nil && opts.Description != "" {
				out.values[string(vd.Name())] = opts.Description
			}
		}
	}
	return out
}

// enumSchema returns a JSON Schema for a protobuf enum field.
func enumSchema(fd protoreflect.FieldDescriptor) map[string]any {
	ed := fd.Enum()
	vals := make([]string, ed.Values().Len())
	for i := range vals {
		vals[i] = string(ed.Values().Get(i).Name())
	}
	schema := map[string]any{"type": "string", "enum": vals}
	descs := getEnumDescriptions(ed)
	if descs.enumDesc != "" || len(descs.values) > 0 {
		var parts []string
		if descs.enumDesc != "" {
			parts = append(parts, strings.TrimSuffix(descs.enumDesc, "."))
		}
		for _, v := range vals {
			if d, ok := descs.values[v]; ok {
				parts = append(parts, fmt.Sprintf("%s: %s", v, d))
			}
		}
		schema["description"] = strings.Join(parts, ". ")
		if len(descs.values) > 0 {
			schema["enumDescriptions"] = descs.values
		}
	}
	return schema
}

// scalarSchema returns a JSON Schema for a protobuf scalar field.
func scalarSchema(fd protoreflect.FieldDescriptor, openAI bool) map[string]any {
	s := map[string]any{"type": kindToType(fd.Kind())}
	if fd.Kind() == protoreflect.BytesKind {
		s["contentEncoding"] = "base64"
		if !openAI {
			s["format"] = "byte"
		}
	}
	return s
}

var strippedCommentPrefixes = []string{"buf:lint:", "@ignore-comment"}

// CleanComment strips annotation prefixes that should not appear in MCP tool descriptions.
func CleanComment(comment string) string {
	var out []string
outer:
	for _, line := range strings.Split(comment, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range strippedCommentPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				continue outer
			}
		}
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(out, " "))
}
