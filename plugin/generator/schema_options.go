// Copyright 2026 The Protobuf Project authors.
// SPDX-License-Identifier: Apache-2.0

package generator

// schema_options.go is what a deployment may trade of a tool's schema for
// its size -- an agent reads every tool's schema into its context, and a
// vocabulary with code lists of hundreds of values and paragraphs of
// documentation on every field makes that expensive. Each is off by default,
// so a schema states everything unless a plugin option says otherwise:
//
//	max_enum_values=64      an enum with more values is a pattern on their
//	                        common prefix, with examples, not the full list
//	short_descriptions=true below the request and the messages it holds
//	                        directly, a field's description is its first
//	                        sentence
//	output_schema=false     tools declare no outputSchema (optional in MCP)
//
// The server validates every request whatever its schema says, so a value a
// pattern admits but the enum lacks is still refused, with the reason.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// SchemaOptions are the size trade-offs a deployment opts into.
type SchemaOptions struct {
	// MaxEnumValues bounds the enums written out in full; 0 writes all.
	MaxEnumValues int
	// ShortDescriptions keeps a nested field's description to its first
	// sentence.
	ShortDescriptions bool
	// NoOutputSchema leaves outputSchema off every tool.
	NoOutputSchema bool
}

// Schemas are the options of this run, set from the plugin's parameters.
var Schemas SchemaOptions

// enumExamples is how many values a pattern for a large enum shows.
const enumExamples = 5

// shortDepth is the message depth from which ShortDescriptions applies: the
// request's own fields (0) and those of the messages it holds (1) keep theirs.
const shortDepth = 2

// outputSchemaFor is md's schema as a tool's outputSchema, as JSON; empty
// when the run declares none, which the templates read as "no outputSchema".
func outputSchemaFor(md protoreflect.MessageDescriptor) string {
	if Schemas.NoOutputSchema {
		return ""
	}
	out, err := json.Marshal(messageSchema(md, false, ""))
	if err != nil {
		panic(fmt.Sprintf("marshal output schema: %v", err))
	}
	return string(out)
}

// largeEnumSchema is the schema of an enum too large to list: a string
// matching the values' common prefix, with a few of them as examples.
func largeEnumSchema(fullName string, vals []string) map[string]any {
	prefix := commonPrefix(vals)
	var examples []any
	for _, v := range vals {
		if len(examples) == enumExamples {
			break
		}
		if !strings.HasSuffix(v, "_UNSPECIFIED") {
			examples = append(examples, v)
		}
	}
	return map[string]any{
		"type":     "string",
		"pattern":  "^" + regexp.QuoteMeta(prefix) + "[A-Za-z0-9_]+$",
		"examples": examples,
		"description": "One of the " + strconv.Itoa(len(vals)) + " values of " + fullName +
			" (too many to list here); the server refuses any other.",
	}
}

// commonPrefix is the values' longest common prefix, cut back to its last
// underscore, so it names the enum rather than part of a value.
func commonPrefix(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	p := vals[0]
	for _, v := range vals[1:] {
		for !strings.HasPrefix(v, p) {
			p = p[:len(p)-1]
		}
	}
	return p[:strings.LastIndex(p, "_")+1]
}

// sentenceEnd is a full stop ending a sentence: followed by a space and a
// capital, and not one of "e.g.", "i.e.", "etc." or an initial.
var sentenceEnd = regexp.MustCompile(`[^.]\.\s+[A-Z(]`)

// firstSentence is d up to and including its first full stop.
func firstSentence(d string) string {
	for _, loc := range sentenceEnd.FindAllStringIndex(d, -1) {
		end := loc[0] + 2 // through the full stop
		before := strings.ToLower(d[max(0, end-5):end])
		if strings.HasSuffix(before, "e.g.") || strings.HasSuffix(before, "i.e.") ||
			strings.HasSuffix(before, "etc.") || (end >= 3 && d[end-3] == ' ') {
			continue
		}
		return d[:end]
	}
	return d
}
