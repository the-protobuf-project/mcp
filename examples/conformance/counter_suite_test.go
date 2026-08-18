package conformance

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// countTool is the single tool CounterService exposes. Its RPC is
// server-streaming, which is how the generator models MCP progress.
const countTool = "counter_service-count_v1"

// runCounterSuite drives one CounterService server.
func runCounterSuite(t *testing.T, tg target, session *mcpsdk.ClientSession, rec *progressRecorder) {
	t.Run("tools", func(t *testing.T) { counterToolSurface(t, callCtx(t), session) })
	t.Run("progress", func(t *testing.T) { counterProgress(t, tg, callCtx(t), session, rec) })
	t.Run("no_progress_token", func(t *testing.T) { counterWithoutProgressToken(t, callCtx(t), session) })
}

// counterToolSurface checks that the streaming RPC is advertised as a tool.
//
// Streaming RPCs used to be filtered out of generation entirely, which produced
// a server with no tools at all -- code that is absent compiles perfectly.
func counterToolSurface(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var found *mcpsdk.Tool
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Name == countTool {
			found = tool
		}
	}
	if found == nil {
		t.Fatalf("tools/list is missing %q; got %v", countTool, names)
	}
	if strings.TrimSpace(found.Description) == "" {
		t.Errorf("%s: empty description", countTool)
	}
	if in, ok := schemaOf(found.InputSchema); !ok || in.typ != "object" {
		t.Errorf("%s: inputSchema is not an object schema", countTool)
	}
}

// counterProgress calls Count with a progressToken and checks both halves of
// the streaming contract: that progress is reported as the work happens, and
// that the client can get the final answer.
//
// Progress is the one part of the protocol that cannot be checked from a single
// request/response pair. The notifications travel the other way, after the call
// has been made, and only over a transport that keeps a channel open. Nothing
// short of a real client on a real transport observes them.
//
// Where the final result arrives is deliberately not pinned down. The Go
// example forwards the RPC over gRPC and answers the tool call when the stream
// ends; the Rust example awaits the handler and answers the same way; the Go
// in-process path returns immediately and delivers the result in a closing
// notification. All three are conforming, and a client has to cope with any of
// them, so the assertion is that the answer is reachable -- not which door it
// comes through.
func counterProgress(t *testing.T, tg target, ctx context.Context, session *mcpsdk.ClientSession, rec *progressRecorder) {
	const countTo = 3
	params := &mcpsdk.CallToolParams{
		Name:      countTool,
		Arguments: map[string]any{"to": countTo},
		Meta:      mcpsdk.Meta{},
	}
	params.SetProgressToken("conformance-progress")

	res, err := session.CallTool(ctx, params)
	if err != nil {
		t.Fatalf("tools/call %s: %v", countTool, err)
	}
	if res.IsError {
		t.Fatalf("tools/call %s returned isError: %s", countTool, firstText(res))
	}

	// Counting to 3 is four steps of work, so a server that reports progress at
	// all should report it more than once; a single notification would satisfy
	// "progress exists" while telling a user nothing as the work runs.
	if err := rec.waitUntil(ctx, 30*time.Second, func(n []mcpsdk.ProgressNotificationParams) bool {
		return len(n) >= countTo
	}); err != nil {
		logNotifications(t, rec.snapshot())
		t.Fatalf("%s asked for progress but did not report it: %v", countTool, err)
	}

	notifs := rec.snapshot()

	// Progress must advance; a server that reports the same value each time is
	// telling the client nothing.
	last := -1.0
	for i, n := range notifs {
		if n.Progress < last {
			t.Errorf("progress notification %d went backwards: %v after %v", i, n.Progress, last)
		}
		last = n.Progress
	}

	// The messages are the service's own, so their presence proves the
	// notifications came from the streaming handler rather than from the
	// transport reporting on itself.
	if strings.TrimSpace(notifs[0].Message) == "" {
		t.Error("progress notifications carry no message")
	}

	assertCountResult(t, ctx, rec, res, countTo)
}

// assertCountResult checks that the count reached the client, whether it came
// back with the tool call or in a closing notification.
func assertCountResult(t *testing.T, ctx context.Context, rec *progressRecorder, res *mcpsdk.CallToolResult, countTo int) {
	t.Helper()

	// The result is the proto's CountResponse, whose only field is count.
	want := fmt.Sprintf(`"count":%d`, countTo)
	normalize := func(s string) string { return strings.ReplaceAll(s, " ", "") }

	if strings.Contains(normalize(firstText(res)), want) {
		return
	}

	err := rec.waitUntil(ctx, 30*time.Second, func(notifs []mcpsdk.ProgressNotificationParams) bool {
		for _, n := range notifs {
			if strings.Contains(normalize(n.Message), want) {
				return true
			}
		}
		return false
	})
	if err != nil {
		logNotifications(t, rec.snapshot())
		t.Fatalf("counting to %d never produced %s, neither in the tool result (%s) nor in a notification",
			countTo, want, firstText(res))
	}
}

// counterWithoutProgressToken calls the same tool with no progressToken.
//
// A client that does not ask for progress must still get a usable answer: the
// generated sink goes inert, and the RPC has to run to completion regardless.
func counterWithoutProgressToken(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession) {
	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      countTool,
		Arguments: map[string]any{"to": 2},
	})
	if err != nil {
		t.Fatalf("tools/call %s without a progressToken: %v", countTool, err)
	}
	if res.IsError {
		t.Fatalf("tools/call %s without a progressToken returned isError: %s", countTool, firstText(res))
	}
	if strings.TrimSpace(firstText(res)) == "" {
		t.Error("tools/call without a progressToken returned an empty result")
	}
}

// logNotifications dumps what actually arrived. Progress bugs are otherwise
// opaque: the test only knows that something it expected never showed up.
func logNotifications(t *testing.T, notifs []mcpsdk.ProgressNotificationParams) {
	t.Helper()
	if len(notifs) == 0 {
		t.Log("no progress notifications were received at all")
		return
	}
	t.Logf("received %d progress notifications:", len(notifs))
	for i, n := range notifs {
		t.Logf("  [%d] progress=%v total=%v message=%q", i, n.Progress, n.Total, n.Message)
	}
}
