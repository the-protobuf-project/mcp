// Package conformance drives every example server in this repository with a
// real MCP client, over a real transport, in a separate process.
//
// The per-language example tests that live next to each server check the
// generated code from inside its own process: they register the handlers on an
// in-memory server and talk to it over an in-memory pipe. That proves the
// generated Go compiles and dispatches, but it cannot see anything the binary
// only does when it actually runs -- which transport it ends up serving, which
// tools the process advertises after start-up, whether a handler reaches the
// gRPC backend behind it. The C++ example compiled and linked cleanly for
// months while advertising an empty tool list.
//
// So this suite deliberately treats each server as a black box: spawn the
// shipped binary, speak MCP to it the way Claude Desktop or MCP Inspector
// would, and assert on the protocol responses alone. Nothing here imports the
// generated packages, which is what lets one suite hold Go, Rust and C++ to a
// single contract.
package conformance

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectTimeout bounds a single connection attempt. Server start-up is
// dominated by process exec plus, for the HTTP targets, binding a port; the C++
// binary additionally sleeps 200ms between starting gRPC and attaching MCP.
const connectTimeout = 30 * time.Second

// progressRecorder collects the progress notifications a session receives.
// Streaming tools report through notifications that arrive after the tool call
// has already returned, so the recorder is attached to the client up front and
// read once the test knows what it is waiting for.
type progressRecorder struct {
	mu     sync.Mutex
	notifs []mcpsdk.ProgressNotificationParams
}

func newProgressRecorder() *progressRecorder { return &progressRecorder{} }

func (p *progressRecorder) record(params *mcpsdk.ProgressNotificationParams) {
	p.mu.Lock()
	p.notifs = append(p.notifs, *params)
	p.mu.Unlock()
}

func (p *progressRecorder) snapshot() []mcpsdk.ProgressNotificationParams {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]mcpsdk.ProgressNotificationParams(nil), p.notifs...)
}

// waitUntil blocks until pred is satisfied by the notifications received so
// far, or the deadline passes.
//
// A predicate rather than a fixed sentinel, because the runtimes signal the end
// of a stream differently -- one closes with a progress==total update, another
// with the Total=1.0 marker its runtime reserves for completion -- and the suite
// should describe what it needs rather than assume one runtime's convention.
func (p *progressRecorder) waitUntil(ctx context.Context, timeout time.Duration, pred func([]mcpsdk.ProgressNotificationParams) bool) error {
	deadline := time.Now().Add(timeout)
	for {
		if pred(p.snapshot()) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met after %s (%d notifications received)",
				timeout, len(p.snapshot()))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// elicitCounter records how many times a server asked the client to confirm
// something. Tools that declare mcp.elicitation are supposed to ask before they
// act, and the only way to observe that from outside is to count the requests
// arriving at the client.
type elicitCounter struct {
	mu sync.Mutex
	n  int
}

func (e *elicitCounter) inc() {
	e.mu.Lock()
	e.n++
	e.mu.Unlock()
}

func (e *elicitCounter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.n
}

// sessionDeps are the observers attached to a session's client, kept by the
// caller so the assertions can read what arrived out of band.
type sessionDeps struct {
	progress *progressRecorder
	elicits  *elicitCounter
}

func newSessionDeps() *sessionDeps {
	return &sessionDeps{progress: newProgressRecorder(), elicits: &elicitCounter{}}
}

// newClient builds the MCP client every target is driven with.
//
// The elicitation handler always accepts. Three of the todo tools declare
// mcp.elicitation in the proto, and the two runtimes implement it differently:
// the Go runtime returns an input-required result the SDK's multi-round-trip
// middleware fulfils and retries, while rmcp issues a server-to-client
// elicitation/create request. Both paths land in this one handler, so the same
// client drives both without knowing which it is talking to.
func newClient(deps *sessionDeps) *mcpsdk.Client {
	opts := &mcpsdk.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcpsdk.ElicitRequest) (*mcpsdk.ElicitResult, error) {
			deps.elicits.inc()
			// "yes" is the enum value the proto's confirmation messages declare;
			// the runtimes map it back onto the CONFIRM_ACTION_YES enum.
			return &mcpsdk.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"confirm": "yes"},
			}, nil
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcpsdk.ProgressNotificationClientRequest) {
			deps.progress.record(req.Params)
		},
	}
	return mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "grpc-mcp-gateway-conformance",
		Version: "0.1.0",
	}, opts)
}

// serverProcess is a spawned example server plus whatever it wrote to stderr.
// Every example logs to stderr (stdout belongs to the stdio transport), so the
// captured buffer is the only diagnostic available when a target fails to come
// up, and it is dumped into the test log on failure.
type serverProcess struct {
	cmd    *exec.Cmd
	stderr *syncBuffer
}

// syncBuffer is an io.Writer safe for the concurrent writes os/exec performs
// from its copying goroutine while a test reads the buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// dumpStderr reports the server's stderr. Called on failure paths only.
func (s *serverProcess) dumpStderr(t *testing.T) {
	t.Helper()
	if out := s.stderr.String(); out != "" {
		t.Logf("--- server stderr ---\n%s---------------------", out)
	}
}

// callCtx bounds a single group of protocol calls.
//
// Every request goes out with a deadline so that a server which accepts a
// request and never answers fails its own subtest, naming itself, instead of
// stalling the run until the package-level timeout kills everything and reports
// nothing. That is not hypothetical: an elicitation request that never reaches
// the client leaves the tool call hanging with no error on either side.
func callCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// connectStdio spawns bin over the stdio transport and returns a live session.
//
// The stdio transport is the one every target supports and the one MCP CLI
// clients use, so it carries the bulk of the suite: no ports, no readiness
// polling, and the process is torn down deterministically when the session
// closes.
func connectStdio(t *testing.T, ctx context.Context, bin string, env []string, deps *sessionDeps) *mcpsdk.ClientSession {
	t.Helper()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	proc := &serverProcess{cmd: cmd, stderr: stderr}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	session, err := newClient(deps).Connect(connectCtx, &mcpsdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		proc.dumpStderr(t)
		t.Fatalf("connect over stdio to %s: %v", bin, err)
	}
	t.Cleanup(func() {
		// Closing the session closes the child's stdin, which is how the stdio
		// transport asks the server to exit; CommandTransport then waits for it
		// and escalates to a signal if it does not.
		if err := session.Close(); err != nil && !isExpectedShutdownErr(err) {
			t.Logf("closing stdio session for %s: %v", bin, err)
		}
	})
	return session
}

// connectHTTP spawns bin as a streamable-HTTP server and returns a live session.
//
// port is chosen by the caller and passed to the server through env; the server
// binds it asynchronously, so this polls the port before handing the endpoint to
// the SDK rather than racing the bind.
func connectHTTP(t *testing.T, ctx context.Context, bin string, env []string, endpoint string, port int, deps *sessionDeps) *mcpsdk.ClientSession {
	t.Helper()

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	proc := &serverProcess{cmd: cmd, stderr: stderr}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	t.Cleanup(func() {
		// An HTTP server has no in-band shutdown, so it is signalled directly.
		// Wait reaps it so the port is free before the next target starts.
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	if err := waitForPort(ctx, port, connectTimeout); err != nil {
		proc.dumpStderr(t)
		t.Fatalf("%s never listened on port %d: %v", bin, port, err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	session, err := newClient(deps).Connect(connectCtx, &mcpsdk.StreamableClientTransport{
		Endpoint: endpoint,
	}, nil)
	if err != nil {
		proc.dumpStderr(t)
		t.Fatalf("connect over streamable-http to %s: %v", endpoint, err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil && !isExpectedShutdownErr(err) {
			t.Logf("closing http session for %s: %v", endpoint, err)
		}
	})
	return session
}

// isExpectedShutdownErr reports whether err is the ordinary noise of tearing
// down a session whose peer is already gone. Treating these as failures would
// make every target flaky at cleanup time without saying anything about the
// server's conformance.
func isExpectedShutdownErr(err error) bool {
	msg := err.Error()
	for _, s := range []string{
		"file already closed",
		"broken pipe",
		"connection reset",
		"EOF",
		"signal: killed",
		"context canceled",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// waitForPort blocks until something accepts connections on port.
func waitForPort(ctx context.Context, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// freePort returns a port that was unbound a moment ago.
//
// There is no way to hand an already-bound listener to a child process here, so
// the port is released before the server claims it. Nothing else in the suite
// binds ports concurrently -- the HTTP targets run serially, and two of the
// example servers hard-code their gRPC port -- so the window is not contended.
func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := lis.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}
	return port
}
