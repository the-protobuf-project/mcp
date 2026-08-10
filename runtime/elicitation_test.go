package runtime

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var confirmFields = []ElicitField{
	{
		Name:        "confirm",
		Description: "Confirm action.",
		Required:    true,
		Type:        "string",
		EnumValues:  []string{"yes", "no"},
		ProtoValues: []string{"CONFIRM_ACTION_YES", "CONFIRM_ACTION_NO"},
	},
}

// The first pass of a tool call has no answer yet, so RunElicitation must ask
// for one via an InputRequests map rather than by calling the client directly:
// protocol version 2026-07-28 rejects server-initiated elicitation requests
// (SEP-2322).
func TestRunElicitation_FirstPassAsksForInput(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tool"}}

	result, pending, err := RunElicitation(req, "Are you sure?", confirmFields)
	if err != nil {
		t.Fatalf("RunElicitation: %v", err)
	}
	if result != nil {
		t.Fatalf("expected no result on the first pass, got %+v", result)
	}
	if pending == nil {
		t.Fatal("expected a pending result asking for input")
	}
	if len(pending.Content) != 0 {
		t.Errorf("pending result must carry no content, got %v", pending.Content)
	}

	ir, ok := pending.InputRequests[ElicitRequestID]
	if !ok {
		t.Fatalf("no input request under %q, got %v", ElicitRequestID, pending.InputRequests)
	}
	params, ok := ir.(*mcp.ElicitParams)
	if !ok {
		t.Fatalf("input request is %T, want *mcp.ElicitParams", ir)
	}
	if params.Message != "Are you sure?" {
		t.Errorf("message: got %q", params.Message)
	}
	sch, ok := params.RequestedSchema.(*jsonschema.Schema)
	if !ok {
		t.Fatalf("requested schema is %T, want *jsonschema.Schema", params.RequestedSchema)
	}
	if sch.Type != "object" {
		t.Errorf("schema type: got %q, want object", sch.Type)
	}
	if len(sch.Required) != 1 || sch.Required[0] != "confirm" {
		t.Errorf("required: got %v, want [confirm]", sch.Required)
	}
	prop, ok := sch.Properties["confirm"]
	if !ok {
		t.Fatalf("no confirm property, got %v", sch.Properties)
	}
	// Enum values are the friendly names, not the proto enum names.
	if len(prop.Enum) != 2 || prop.Enum[0] != "yes" || prop.Enum[1] != "no" {
		t.Errorf("enum: got %v, want [yes no]", prop.Enum)
	}
}

// On the retry the client echoes the answer back in InputResponses, and
// RunElicitation hands it to the handler instead of asking again.
func TestRunElicitation_RetryReturnsAnswer(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name: "tool",
		InputResponses: mcp.InputResponseMap{
			ElicitRequestID: &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"confirm": "yes"},
			},
		},
	}}

	result, pending, err := RunElicitation(req, "Are you sure?", confirmFields)
	if err != nil {
		t.Fatalf("RunElicitation: %v", err)
	}
	if pending != nil {
		t.Fatalf("expected no pending result on the retry, got %+v", pending)
	}
	if result == nil {
		t.Fatal("expected the client's answer")
	}
	if result.Action != "accept" {
		t.Errorf("action: got %q, want accept", result.Action)
	}
	if got := result.Content["confirm"]; got != "yes" {
		t.Errorf("confirm: got %v, want yes", got)
	}
}

func TestRunElicitation_DeclinedAnswer(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:           "tool",
		InputResponses: mcp.InputResponseMap{ElicitRequestID: &mcp.ElicitResult{Action: "decline"}},
	}}

	result, pending, err := RunElicitation(req, "Are you sure?", confirmFields)
	if err != nil || pending != nil {
		t.Fatalf("RunElicitation: result=%v pending=%v err=%v", result, pending, err)
	}
	if result.Action != "decline" {
		t.Errorf("action: got %q, want decline", result.Action)
	}
}

func TestRunElicitation_WrongResponseType(t *testing.T) {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{
		Name:           "tool",
		InputResponses: mcp.InputResponseMap{ElicitRequestID: nil},
	}}

	if _, _, err := RunElicitation(req, "Are you sure?", confirmFields); err == nil {
		t.Fatal("expected an error for a non-elicitation input response")
	}
}

// MergeElicitResult overlays the answer onto the LLM's arguments, mapping the
// friendly enum name the form showed back to its proto enum name.
func TestMergeElicitResult_ReverseMapsEnum(t *testing.T) {
	args := json.RawMessage(`{"parent":"users/alice"}`)

	merged := MergeElicitResult(args, map[string]any{"confirm": "yes"}, confirmFields)

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal merged args: %v", err)
	}
	if got["parent"] != "users/alice" {
		t.Errorf("parent: got %v, want users/alice", got["parent"])
	}
	if got["confirm"] != "CONFIRM_ACTION_YES" {
		t.Errorf("confirm: got %v, want CONFIRM_ACTION_YES", got["confirm"])
	}
}
