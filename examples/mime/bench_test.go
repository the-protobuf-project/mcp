package main

import (
	"encoding/json"
	"testing"

	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

// The generated handler marshals the proto response to JSON, then has to hand
// that JSON to the SDK as structuredContent. Parsing it into a generic map costs
// a second full pass over every response; json.RawMessage hands the same bytes
// through untouched, since the SDK marshals structuredContent without inspecting
// it.
var benchPayload = func() []byte {
	// A response of realistic shape: the gallery's manifest listing.
	b, err := json.Marshal(map[string]any{
		"assets": func() []any {
			out := make([]any, 0, 6)
			for _, a := range assets {
				out = append(out, map[string]any{
					"id": a.id, "title": a.title, "uri": a.uri,
					"mime_type": a.mimeType, "size_bytes": a.sizeBytes(),
				})
			}
			return out
		}(),
	})
	if err != nil {
		panic(err)
	}
	return b
}()

func BenchmarkStructuredContentParsed(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		result := mcp.TextResult(string(benchPayload))
		var structured any
		if err := json.Unmarshal(benchPayload, &structured); err != nil {
			b.Fatal(err)
		}
		result.StructuredContent = structured
		_ = result
	}
}

func BenchmarkStructuredContentRaw(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		result := mcp.TextResult(string(benchPayload))
		result.StructuredContent = json.RawMessage(benchPayload)
		_ = result
	}
}

// Both must produce byte-identical JSON on the wire, or the optimisation is a
// behaviour change rather than a speedup.
func TestStructuredContentEncodingsMatch(t *testing.T) {
	var parsed any
	if err := json.Unmarshal(benchPayload, &parsed); err != nil {
		t.Fatal(err)
	}
	fromParsed, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	fromRaw, err := json.Marshal(json.RawMessage(benchPayload))
	if err != nil {
		t.Fatal(err)
	}
	if string(fromParsed) != string(fromRaw) {
		t.Errorf("encodings differ:\n parsed: %s\n raw:    %s", fromParsed, fromRaw)
	}
}
