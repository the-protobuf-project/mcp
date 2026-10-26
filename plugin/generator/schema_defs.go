// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

// schema_defs.go keeps a tool's schema the size of its type graph rather
// than of every path through it.
//
// Inlining a message at each field that holds it repeats a shared type once
// per path to it: a vocabulary like OpenTravel's, where a PersonName or an
// Address sits under dozens of messages that sit under dozens more, grows a
// single tool's schema to megabytes and a service's tools/list past what an
// MCP client will read. So a message used from more than one field of the
// graph -- or on a cycle, which inlining could only cut -- is written once,
// under the root's $defs, and each field that holds it is a $ref there (the
// field's own description and rules stay beside the $ref, as JSON Schema
// 2020-12 allows). A message held by a single field stays inline, so a small
// schema reads as it always did.
//
// The same message renders two ways inside an input: normally, and beneath
// an IGNORE_ALWAYS field (see unruledInput) with no rules and no required
// fields. Each way is its own definition.

import "google.golang.org/protobuf/reflect/protoreflect"

// wellKnown are the message types messageFieldSchemaPath writes as a JSON
// value of their own, never as an object of their fields.
var wellKnown = map[protoreflect.FullName]bool{
	"google.protobuf.Timestamp": true, "google.protobuf.Duration": true,
	"google.protobuf.Struct": true, "google.protobuf.Value": true, "google.protobuf.ListValue": true,
	"google.protobuf.FieldMask": true, "google.protobuf.Any": true,
	"google.protobuf.DoubleValue": true, "google.protobuf.FloatValue": true,
	"google.protobuf.Int32Value": true, "google.protobuf.UInt32Value": true,
	"google.protobuf.Int64Value": true, "google.protobuf.UInt64Value": true,
	"google.protobuf.StringValue": true, "google.protobuf.BoolValue": true, "google.protobuf.BytesValue": true,
}

// defKey is a message as it renders in one context.
type defKey struct {
	name    protoreflect.FullName
	unruled bool
}

// ref is the $defs entry's name: the message's full name, marked when it is
// the rule-free rendering.
func (k defKey) ref() string {
	if k.unruled {
		return string(k.name) + "@partial"
	}
	return string(k.name)
}

// sharedDefs walks root's type graph -- each message once per context -- and
// returns the messages to write under $defs: those held by two or more
// fields of the graph, and those on a cycle.
func sharedDefs(root protoreflect.MessageDescriptor, input bool) map[defKey]bool {
	a := &analysis{input: input, sites: map[defKey]int{}, seen: map[defKey]bool{}, stack: map[defKey]bool{}, shared: map[defKey]bool{}}
	a.visit(defKey{name: root.FullName()}, root)
	for k, n := range a.sites {
		if n > 1 {
			a.shared[k] = true
		}
	}
	return a.shared
}

// analysis is one sharedDefs walk.
type analysis struct {
	input  bool
	sites  map[defKey]int  // fields of the graph holding each message
	seen   map[defKey]bool // messages whose fields were walked
	stack  map[defKey]bool // messages open on the current branch
	shared map[defKey]bool // the result: messages on a cycle, then shared ones
}

// visit walks md's fields, counting each message-typed one as a site of the
// message it holds, as fieldSchemaPath would render it.
func (a *analysis) visit(k defKey, md protoreflect.MessageDescriptor) {
	if a.stack[k] {
		a.shared[k] = true
		return
	}
	if a.seen[k] {
		return
	}
	a.seen[k], a.stack[k] = true, true
	defer delete(a.stack, k)
	w := &walk{input: a.input}
	for i := 0; i < md.Fields().Len(); i++ {
		fd := md.Fields().Get(i)
		if a.input && isOutputOnly(fd) {
			continue
		}
		held := fd
		if fd.IsMap() {
			held = fd.MapValue()
		}
		if held.Kind() != protoreflect.MessageKind || wellKnown[held.Message().FullName()] {
			continue
		}
		child := defKey{name: held.Message().FullName(), unruled: k.unruled || unruledInput(fd, w)}
		a.sites[child]++
		a.visit(child, held.Message())
	}
}
