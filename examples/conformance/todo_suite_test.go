package conformance

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// todoTools is the tool surface every TodoService server must advertise.
//
// The names are the generator's, derived from the proto: service name, method
// name, and package version. Asserting the exact set (not a subset) is what
// makes this a contract -- a tool silently dropped by one generator, or an
// extra one appearing, fails here rather than being discovered by a user.
var todoTools = []string{
	"todo_service-create_todo_v1",
	"todo_service-delete_todo_v1",
	"todo_service-get_todo_v1",
	"todo_service-list_todos_v1",
	"todo_service-update_todo_v1",
}

// todoRequiredArgs is the required-argument set each tool's inputSchema must
// declare, taken from google.api.method_signature and field_behavior in the
// proto. A client picks arguments from this schema, so a drift here means an
// LLM is being told the wrong thing about how to call the RPC.
var todoRequiredArgs = map[string][]string{
	"todo_service-create_todo_v1": {"parent", "todo", "todo_id"},
	"todo_service-delete_todo_v1": {"name"},
	"todo_service-get_todo_v1":    {"name"},
	"todo_service-list_todos_v1":  {"parent"},
	"todo_service-update_todo_v1": {"todo"},
}

// runTodoSuite drives one TodoService server through the whole contract.
// The subtests share one session, so order matters: anything that can leave the
// session unusable runs last. "errors" is deliberately at the end because one
// transport tears the session down when a tool reports failure, and a cascade of
// unrelated red subtests buries the finding that caused it.
func runTodoSuite(t *testing.T, tg target, session *mcpsdk.ClientSession, elicits *elicitCounter) {
	t.Run("tools", func(t *testing.T) { todoToolSurface(t, callCtx(t), session) })

	// A read-only call, so it holds even where the mutating tools cannot run.
	// Without it, a target excused from "crud" would never exercise tools/call
	// at all and would pass on tools/list alone.
	t.Run("read_only_call", func(t *testing.T) { todoReadOnlyCall(t, callCtx(t), session) })

	t.Run("crud", func(t *testing.T) {
		if tg.hasGap(gapHTTPElicitationHangs) {
			t.Skipf("target declares gap %q: every mutating todo tool declares mcp.elicitation", gapHTTPElicitationHangs)
		}
		todoCRUD(t, tg, session, elicits)
	})
	t.Run("prompts", func(t *testing.T) {
		if tg.hasGap(gapNoPrompts) {
			t.Skipf("target declares gap %q", gapNoPrompts)
		}
		todoPrompts(t, callCtx(t), session)
	})
	t.Run("resources", func(t *testing.T) {
		if tg.hasGap(gapNoResources) {
			t.Skipf("target declares gap %q", gapNoResources)
		}
		todoResources(t, callCtx(t), session)
	})
	t.Run("completion", func(t *testing.T) {
		if tg.hasGap(gapNoCompletion) {
			t.Skipf("target declares gap %q", gapNoCompletion)
		}
		todoCompletion(t, callCtx(t), session)
	})
	t.Run("errors", func(t *testing.T) {
		if tg.hasGap(gapHTTPErrorClosesSession) {
			t.Skipf("target declares gap %q", gapHTTPErrorClosesSession)
		}
		todoErrors(t, tg, callCtx(t), session)
	})
}

// todoReadOnlyCall proves the tools/call path reaches the service, using the one
// todo tool that declares no elicitation and mutates nothing.
func todoReadOnlyCall(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-list_todos_v1",
		Arguments: map[string]any{"parent": "users/nobody"},
	})
	if err != nil {
		t.Fatalf("tools/call todo_service-list_todos_v1: %v", err)
	}
	if res.IsError {
		t.Fatalf("listing an empty parent reported an error: %s", firstText(res))
	}
	// The response is a ListTodosResponse; an empty list still serialises to an
	// object, so a blank body means the call did not reach the service.
	if strings.TrimSpace(firstText(res)) == "" {
		t.Error("tools/call returned an empty body")
	}
}

// todoToolSurface checks tools/list: the exact tool set, and that each entry
// carries the metadata a client needs to call it.
func todoToolSurface(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	got := make([]string, 0, len(res.Tools))
	byName := map[string]*mcpsdk.Tool{}
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
		byName[tool.Name] = tool
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(todoTools, ",") {
		t.Fatalf("tools/list =\n  %v\nwant\n  %v", got, todoTools)
	}

	for _, name := range todoTools {
		tool := byName[name]
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("%s: empty description; the proto declares one via mcp.tool", name)
		}
		in, ok := schemaOf(tool.InputSchema)
		if !ok {
			t.Errorf("%s: no inputSchema", name)
			continue
		}
		if in.typ != "object" {
			t.Errorf("%s: inputSchema.type = %q, want object", name, in.typ)
		}
		gotReq := append([]string(nil), in.required...)
		sort.Strings(gotReq)
		wantReq := append([]string(nil), todoRequiredArgs[name]...)
		sort.Strings(wantReq)
		if strings.Join(gotReq, ",") != strings.Join(wantReq, ",") {
			t.Errorf("%s: inputSchema.required = %v, want %v", name, gotReq, wantReq)
		}
		if _, ok := schemaOf(tool.OutputSchema); !ok {
			t.Errorf("%s: no outputSchema", name)
		}
	}
}

// todoCRUD runs a create/get/list/update/delete round trip and checks that each
// call actually reached the service behind the MCP layer.
//
// This is the assertion the build could never make. Every server here puts a
// different mechanism between the tool call and the store -- Go dispatches in
// process, the Go counter example and the C++ server forward over gRPC, Rust
// calls an async trait -- and all of it is invisible until a value written by
// one call is read back by the next.
func todoCRUD(t *testing.T, tg target, session *mcpsdk.ClientSession, elicits *elicitCounter) {
	ctx := callCtx(t)
	const (
		parent = "users/alice"
		todoID = "conformance-1"
		name   = parent + "/todos/" + todoID
		title  = "Buy groceries"
	)

	before := elicits.count()

	created := callTool(t, ctx, session, "todo_service-create_todo_v1", map[string]any{
		"parent":  parent,
		"todo_id": todoID,
		"todo": map[string]any{
			"title":       title,
			"description": "Milk, eggs, bread",
			"priority":    "PRIORITY_HIGH",
		},
	})
	requireContains(t, "create", created.text, name, title, "PRIORITY_HIGH")
	assertStructured(t, tg, "create", created)

	// The three mutating tools declare mcp.elicitation, so a conforming server
	// must ask the client to confirm before it writes.
	if !tg.hasGap(gapNoElicitation) && elicits.count() == before {
		t.Errorf("create_todo declares mcp.elicitation in the proto but the server never asked the client to confirm")
	}

	got := callTool(t, ctx, session, "todo_service-get_todo_v1", map[string]any{"name": name})
	requireContains(t, "get", got.text, name, title)
	assertStructured(t, tg, "get", got)

	listed := callTool(t, ctx, session, "todo_service-list_todos_v1", map[string]any{"parent": parent})
	requireContains(t, "list", listed.text, todoID, title)

	updated := callTool(t, ctx, session, "todo_service-update_todo_v1", map[string]any{
		"todo": map[string]any{
			"name":      name,
			"title":     "Buy groceries and cook",
			"completed": true,
		},
		"update_mask": "title,completed",
	})
	requireContains(t, "update", updated.text, "Buy groceries and cook")

	// Read the update back rather than trusting the mutation's own response:
	// an implementation that echoes its request would pass the line above.
	reread := callTool(t, ctx, session, "todo_service-get_todo_v1", map[string]any{"name": name})
	requireContains(t, "get after update", reread.text, "Buy groceries and cook")

	callTool(t, ctx, session, "todo_service-delete_todo_v1", map[string]any{"name": name})

	afterDelete := callTool(t, ctx, session, "todo_service-list_todos_v1", map[string]any{"parent": parent})
	if strings.Contains(afterDelete.text, todoID) {
		t.Errorf("list after delete still contains %s: %s", todoID, afterDelete.text)
	}
}

// todoErrors checks that a failing RPC comes back as an MCP-level failure the
// client can see, rather than as a success carrying an error string or as a
// dropped connection.
//
// The two runtimes report it differently -- the Go runtime returns a result
// with isError set, rmcp returns a JSON-RPC error -- and both are legitimate
// under the spec, so this accepts either and only rejects a call that appears
// to have succeeded.
func todoErrors(t *testing.T, tg target, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "todo_service-get_todo_v1",
		Arguments: map[string]any{"name": "users/alice/todos/does-not-exist"},
	})
	if err != nil {
		return // JSON-RPC error: reported, which is what matters.
	}
	if res.IsError {
		return // isError result: also reported.
	}

	body := firstText(res)
	if tg.hasGap(gapErrorsReportedAsSuccess) {
		// Still worth an assertion: the weaker guarantee is that the failure is
		// at least visible in the body. Returning an empty or default-valued
		// Todo -- a client would read that as "the resource exists" -- is a
		// worse failure than the one being excused, and would be caught here.
		if !strings.Contains(body, "error") {
			t.Errorf("get_todo on a missing resource reported success and did not mention an error: %s", body)
		}
		t.Logf("known gap %q: failure returned as a successful result: %s", gapErrorsReportedAsSuccess, body)
		return
	}
	t.Errorf("get_todo on a missing resource reported success: %s", body)
}

// todoPrompts checks prompts/list and prompts/get against the two mcp.prompt
// options the proto declares.
func todoPrompts(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.ListPrompts(ctx, &mcpsdk.ListPromptsParams{})
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}

	byName := map[string]*mcpsdk.Prompt{}
	for _, p := range res.Prompts {
		byName[p.Name] = p
	}

	// Argument sets come from the schema messages the proto names, so they
	// check that the generator resolved those messages rather than emitting a
	// prompt with no arguments.
	wantArgs := map[string]map[string]bool{ // prompt -> arg -> required
		"summarize_todos":  {"user": true},
		"prioritize_todos": {"user": true, "strategy": false},
	}
	for name, want := range wantArgs {
		p, ok := byName[name]
		if !ok {
			t.Errorf("prompts/list is missing %q; got %v", name, promptNames(res.Prompts))
			continue
		}
		if strings.TrimSpace(p.Description) == "" {
			t.Errorf("prompt %s: empty description", name)
		}
		gotArgs := map[string]bool{}
		for _, a := range p.Arguments {
			gotArgs[a.Name] = a.Required
		}
		for arg, required := range want {
			got, ok := gotArgs[arg]
			if !ok {
				t.Errorf("prompt %s: missing argument %q", name, arg)
				continue
			}
			if got != required {
				t.Errorf("prompt %s: argument %s required = %v, want %v", name, arg, got, required)
			}
		}
	}

	got, err := session.GetPrompt(ctx, &mcpsdk.GetPromptParams{
		Name:      "summarize_todos",
		Arguments: map[string]string{"user": "alice"},
	})
	if err != nil {
		t.Fatalf("prompts/get summarize_todos: %v", err)
	}
	if len(got.Messages) == 0 {
		t.Error("prompts/get summarize_todos returned no messages")
	}
}

// todoResources checks that the app resource declared by mcp.service is listed
// and readable, and that the todo resource template is advertised.
func todoResources(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	const appURI = "ui://todoservice/app.html"
	list, err := session.ListResources(ctx, &mcpsdk.ListResourcesParams{})
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	var app *mcpsdk.Resource
	for _, r := range list.Resources {
		if r.URI == appURI {
			app = r
		}
	}
	if app == nil {
		t.Fatalf("resources/list is missing the app resource %q; got %v", appURI, resourceURIs(list.Resources))
	}
	if app.MIMEType != "text/html" {
		t.Errorf("app resource mimeType = %q, want text/html", app.MIMEType)
	}

	read, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: appURI})
	if err != nil {
		t.Fatalf("resources/read %s: %v", appURI, err)
	}
	if len(read.Contents) == 0 {
		t.Fatalf("resources/read %s returned no contents", appURI)
	}
	// The app name comes from mcp.service in the proto; finding it in the body
	// proves the served document is the generated one.
	if !strings.Contains(read.Contents[0].Text, "Todo App") {
		t.Errorf("app resource body does not mention the app name %q: %s", "Todo App", read.Contents[0].Text)
	}

	tmpls, err := session.ListResourceTemplates(ctx, &mcpsdk.ListResourceTemplatesParams{})
	if err != nil {
		t.Fatalf("resources/templates/list: %v", err)
	}
	const wantTmpl = "todo://users/{user}/todos/{todo}"
	found := false
	for _, tmpl := range tmpls.ResourceTemplates {
		if tmpl.URITemplate == wantTmpl {
			found = true
		}
	}
	if !found {
		t.Errorf("resources/templates/list is missing %q", wantTmpl)
	}
}

// todoCompletion checks completion/complete for the prompt argument whose
// enum_values the proto declares.
func todoCompletion(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.Complete(ctx, &mcpsdk.CompleteParams{
		Ref:      &mcpsdk.CompleteReference{Type: "ref/prompt", Name: "prioritize_todos"},
		Argument: mcpsdk.CompleteParamsArgument{Name: "strategy", Value: ""},
	})
	if err != nil {
		t.Fatalf("completion/complete: %v", err)
	}
	want := map[string]bool{"urgency": true, "deadline": true, "effort": true}
	for _, v := range res.Completion.Values {
		delete(want, v)
	}
	if len(want) > 0 {
		t.Errorf("completion/complete for prioritize_todos:strategy = %v, missing %v",
			res.Completion.Values, keys(want))
	}

	// A prefix must narrow the list, or the server is returning the enum
	// wholesale and ignoring what the user typed.
	pre, err := session.Complete(ctx, &mcpsdk.CompleteParams{
		Ref:      &mcpsdk.CompleteReference{Type: "ref/prompt", Name: "prioritize_todos"},
		Argument: mcpsdk.CompleteParamsArgument{Name: "strategy", Value: "ur"},
	})
	if err != nil {
		t.Fatalf("completion/complete with prefix: %v", err)
	}
	if len(pre.Completion.Values) != 1 || pre.Completion.Values[0] != "urgency" {
		t.Errorf("completion for prefix %q = %v, want [urgency]", "ur", pre.Completion.Values)
	}
}

// --- helpers ---

// toolCall is a tool result reduced to what the assertions look at.
type toolCall struct {
	text       string
	structured any
	raw        *mcpsdk.CallToolResult
}

// callTool invokes a tool and fails the test unless it succeeds.
func callTool(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession, name string, args map[string]any) toolCall {
	t.Helper()
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("tools/call %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("tools/call %s returned isError: %s", name, firstText(res))
	}
	return toolCall{text: firstText(res), structured: res.StructuredContent, raw: res}
}

// assertStructured enforces the outputSchema contract: a tool that advertises
// an output schema must return structuredContent matching it.
func assertStructured(t *testing.T, tg target, label string, call toolCall) {
	t.Helper()
	if tg.hasGap(gapNoStructuredContent) {
		return
	}
	if call.structured == nil {
		t.Errorf("%s: tool declares an outputSchema but the result carries no structuredContent", label)
		return
	}
	if _, ok := call.structured.(map[string]any); !ok {
		t.Errorf("%s: structuredContent is %T, want a JSON object matching the outputSchema", label, call.structured)
	}
}

// requireContains fails when any wanted substring is missing from got.
func requireContains(t *testing.T, label, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s: response does not contain %q\ngot: %s", label, w, got)
		}
	}
}

// firstText returns the text of a result's first content block.
func firstText(res *mcpsdk.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	b, err := json.Marshal(res.Content[0])
	if err != nil {
		return ""
	}
	var block struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &block); err != nil {
		return ""
	}
	return block.Text
}

// schema is the part of a JSON Schema the assertions read.
//
// A client receives schemas as plain decoded JSON rather than as typed values,
// which is the honest shape: the server sent bytes, and any structure imposed
// here is this test's, not the protocol's.
type schema struct {
	typ      string
	required []string
}

// schemaOf reads a schema off a tool, reporting false when there is none.
func schemaOf(v any) (schema, bool) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return schema{}, false
	}
	var s schema
	if typ, ok := m["type"].(string); ok {
		s.typ = typ
	}
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if name, ok := r.(string); ok {
				s.required = append(s.required, name)
			}
		}
	}
	return s, true
}

func promptNames(prompts []*mcpsdk.Prompt) []string {
	out := make([]string, 0, len(prompts))
	for _, p := range prompts {
		out = append(out, p.Name)
	}
	return out
}

func resourceURIs(resources []*mcpsdk.Resource) []string {
	out := make([]string, 0, len(resources))
	for _, r := range resources {
		out = append(out, r.URI)
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
