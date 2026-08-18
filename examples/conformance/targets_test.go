package conformance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Languages the suite knows how to drive.
const (
	langGo   = "go"
	langRust = "rust"
	langCpp  = "cpp"
)

// Services an example server can expose. Each maps to a behavioural suite.
const (
	serviceTodo    = "todo"
	serviceCounter = "counter"
)

// Transports a target is reached over.
const (
	transportStdio = "stdio"
	transportHTTP  = "streamable-http"
)

// Known gaps a target may declare.
//
// Every assertion in the behavioural suites is strict by default; a target opts
// out of one only by naming the gap here. That makes the divergences between
// the three generated runtimes an explicit, reviewable list instead of a set of
// assertions quietly weakened to the lowest common denominator -- and closing a
// gap is done by deleting its entry and watching the suite go green.
const (
	// The Go runtime returns structuredContent alongside the text block for
	// every tool that declares an outputSchema. The Rust and C++ generators
	// advertise the same outputSchema but return text only, so those tools
	// promise a contract their results do not keep.
	gapNoStructuredContent = "structured-content"
	// mcp.prompt is only emitted by the Go and Rust generators.
	gapNoPrompts = "prompts"
	// Resources and resource templates, likewise.
	gapNoResources = "resources"
	// completion/complete, backed by enum_values on prompt arguments.
	gapNoCompletion = "completion"
	// mcp.elicitation. Go implements it as SEP-2322 input-required results;
	// rmcp issues elicitation/create requests. The C++ generator emits neither,
	// so its destructive tools run without asking.
	gapNoElicitation = "elicitation"
	// Elicitation never completes over rmcp's streamable-HTTP transport, so
	// every tool that declares mcp.elicitation hangs until the client's
	// deadline. The same server and the same client complete it over stdio.
	//
	// The server side looks right: it opens the POST response as an SSE stream
	// and writes the elicitation/create request onto it, which is observable
	// with curl. What does not happen is the round trip closing -- the call
	// never returns. Whether the response is not sent or not correlated is an
	// upstream question (rmcp, or the Go SDK's streamable client), so this is
	// recorded rather than worked around here.
	gapHTTPElicitationHangs = "http-elicitation-hangs"
	// A tool error arrives over rmcp's streamable-HTTP as an HTTP 400, which
	// the SDK client treats as a transport failure and closes the session on --
	// taking every later call on that session with it. Over stdio the same
	// error is an ordinary JSON-RPC error response and the session survives.
	gapHTTPErrorClosesSession = "http-error-closes-session"
	// A failed RPC comes back from the C++ adapter as a *successful* tool
	// result whose body is {"error": "..."} -- no isError, no JSON-RPC error.
	// A client is told the call worked and handed an object that does not match
	// the declared outputSchema.
	//
	// The cause is structural: the generated C++ adapter hands the Rust bridge
	// a JSON string across the cxx boundary and has no channel for failure, so
	// the handler wraps whatever it gets in CallToolResult::success. Fixing it
	// means changing both templates, not this test.
	gapErrorsReportedAsSuccess = "errors-reported-as-success"
)

// ports carries the addresses a target's process is given for one run. Both are
// picked per run so that nothing depends on a port being free by convention.
type ports struct {
	mcp  int
	grpc int
}

// target is one (binary, transport, service) triple the suite drives.
//
// The same behavioural suite runs against every target that names a given
// service, which is the point: the servers were generated from one .proto, so
// one set of assertions should hold for all of them regardless of the language
// they were generated into or the transport they are reached over.
type target struct {
	name      string
	lang      string
	service   string
	transport string

	// binary resolves the server executable, returning a reason instead when it
	// is not present. Go builds on demand; the other languages need their own
	// toolchain to have run first.
	binary func(t *testing.T) (path string, missing string)

	// env is the process environment for one run.
	env func(p ports) []string

	// endpoint is the MCP URL for HTTP targets, empty for stdio.
	endpoint func(p ports) string

	// fixedGRPCPort marks targets whose gRPC backend address cannot be moved,
	// so the runner does not bother allocating one.
	fixedGRPCPort bool

	// gaps lists the assertions this target is excused from, each with a
	// comment on the constant explaining why.
	gaps []string
}

// hasGap reports whether the target declared the named gap.
func (tg target) hasGap(gap string) bool {
	for _, g := range tg.gaps {
		if g == gap {
			return true
		}
	}
	return false
}

// examplesDir is the examples module root, one level above this package.
func examplesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve examples dir: %v", err)
	}
	return dir
}

// goBuild memoises one `go build` invocation. Two targets share a command (the
// counter example serves both transports), and the result -- including a
// failure -- has to be reported to every one of them, not just whichever
// happened to run the build.
type goBuild struct {
	once sync.Once
	path string
	err  error
}

var (
	goBuilds    sync.Map // package path -> *goBuild
	goBuildDir  string
	goBuildInit sync.Once
)

// goBinary builds one of the Go example commands and returns the executable.
//
// The Go examples are built here rather than expected on disk because this
// suite already needs a Go toolchain to run at all, so building them adds no
// prerequisite -- and it guarantees the binary under test matches the working
// tree instead of whatever was last compiled.
func goBinary(pkg string) func(t *testing.T) (string, string) {
	return func(t *testing.T) (string, string) {
		t.Helper()

		goBuildInit.Do(func() {
			dir, err := os.MkdirTemp("", "mcp-conformance-bin-")
			if err != nil {
				t.Fatalf("create build dir: %v", err)
			}
			goBuildDir = dir
		})

		entryAny, _ := goBuilds.LoadOrStore(pkg, &goBuild{})
		entry := entryAny.(*goBuild)
		entry.once.Do(func() {
			entry.path = filepath.Join(goBuildDir, strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", "-"))
			cmd := exec.Command("go", "build", "-o", entry.path, pkg)
			cmd.Dir = examplesDir(t)
			if combined, err := cmd.CombinedOutput(); err != nil {
				entry.err = fmt.Errorf("go build %s: %v\n%s", pkg, err, combined)
			}
		})
		if entry.err != nil {
			// A Go example that does not compile is a failure of this repository,
			// not a missing prerequisite, so it is never reported as a skip.
			t.Fatalf("%v", entry.err)
		}
		return entry.path, ""
	}
}

// prebuiltBinary resolves a binary produced by a non-Go toolchain.
//
// envVar lets CI point at a binary somewhere else (a release profile, an
// install prefix). Otherwise the conventional in-tree output paths are tried in
// order, and the first that exists wins.
func prebuiltBinary(envVar, hint string, candidates ...string) func(t *testing.T) (string, string) {
	return func(t *testing.T) (string, string) {
		t.Helper()
		if override := os.Getenv(envVar); override != "" {
			if _, err := os.Stat(override); err != nil {
				return "", fmt.Sprintf("%s=%s does not exist: %v", envVar, override, err)
			}
			return override, ""
		}
		root := examplesDir(t)
		tried := make([]string, 0, len(candidates))
		for _, rel := range candidates {
			p := filepath.Join(root, rel)
			if _, err := os.Stat(p); err == nil {
				return p, ""
			}
			tried = append(tried, rel)
		}
		return "", fmt.Sprintf("no binary at %s (build it with: %s), or set %s",
			strings.Join(tried, " or "), hint, envVar)
	}
}

var (
	rustBinary = func(name string) func(t *testing.T) (string, string) {
		return prebuiltBinary(
			"MCP_CONFORMANCE_RUST_"+strings.ToUpper(name),
			"cd examples/rust && cargo build --bins",
			"rust/target/debug/"+name,
			"rust/target/release/"+name,
		)
	}
	cppBinary = prebuiltBinary(
		"MCP_CONFORMANCE_CPP_SERVER",
		"cd examples/cpp && make",
		"cpp/server",
	)
)

// The MCP paths each server serves, derived by the generator from the proto
// package and service name. They are spelled out rather than imported from the
// generated Go so that a change to the generator shows up here as a failing
// test rather than as two constants moving together silently.
const (
	todoBasePath    = "/todo/v1/todoservice/mcp"
	counterBasePath = "/counter/v1/counterservice/mcp"
)

func httpEndpoint(basePath string) func(p ports) string {
	return func(p ports) string {
		return fmt.Sprintf("http://127.0.0.1:%d%s", p.mcp, basePath)
	}
}

// targets is the full matrix the suite runs.
//
// Every language is driven over stdio, which all three support and which MCP
// CLI clients use. streamable-HTTP is covered once per language that ships an
// HTTP entrypoint, since the transport is shared runtime code and the point is
// to prove the wiring works, not to re-run the same assertions per path.
func targets() []target {
	return []target{
		{
			name:      "go/todo/stdio",
			lang:      langGo,
			service:   serviceTodo,
			transport: transportStdio,
			binary:    goBinary("./go/stdio"),
			env:       func(ports) []string { return nil },
		},
		{
			name:      "go/todo/streamable-http",
			lang:      langGo,
			service:   serviceTodo,
			transport: transportHTTP,
			binary:    goBinary("./go/http"),
			env: func(p ports) []string {
				return []string{
					"MCP_TRANSPORT=streamable-http",
					fmt.Sprintf("MCP_ADDR=:%d", p.mcp),
					fmt.Sprintf("GRPC_ADDR=:%d", p.grpc),
				}
			},
			endpoint: httpEndpoint(todoBasePath),
		},
		{
			// The counter example forwards tool calls to its gRPC backend over a
			// real connection rather than calling the implementation in process,
			// so this target also covers the forwarding path.
			name:      "go/counter/stdio",
			lang:      langGo,
			service:   serviceCounter,
			transport: transportStdio,
			binary:    goBinary("./go/counter"),
			env: func(p ports) []string {
				return []string{
					"MCP_TRANSPORT=stdio",
					fmt.Sprintf("GRPC_ADDR=:%d", p.grpc),
				}
			},
		},
		{
			name:      "go/counter/streamable-http",
			lang:      langGo,
			service:   serviceCounter,
			transport: transportHTTP,
			binary:    goBinary("./go/counter"),
			env: func(p ports) []string {
				return []string{
					"MCP_TRANSPORT=streamable-http",
					fmt.Sprintf("MCP_ADDR=:%d", p.mcp),
					fmt.Sprintf("GRPC_ADDR=:%d", p.grpc),
				}
			},
			endpoint: httpEndpoint(counterBasePath),
		},
		{
			name:      "rust/todo/stdio",
			lang:      langRust,
			service:   serviceTodo,
			transport: transportStdio,
			binary:    rustBinary("stdio"),
			env:       func(ports) []string { return nil },
			gaps:      []string{gapNoStructuredContent},
		},
		{
			name:      "rust/todo/streamable-http",
			lang:      langRust,
			service:   serviceTodo,
			transport: transportHTTP,
			binary:    rustBinary("http"),
			env: func(p ports) []string {
				return []string{
					"MCP_HOST=127.0.0.1",
					fmt.Sprintf("MCP_PORT=%d", p.mcp),
					fmt.Sprintf("GRPC_ADDR=[::]:%d", p.grpc),
				}
			},
			endpoint: httpEndpoint(todoBasePath),
			gaps: []string{
				gapNoStructuredContent,
				gapHTTPElicitationHangs,
				gapHTTPErrorClosesSession,
			},
		},
		{
			name:      "rust/counter/stdio",
			lang:      langRust,
			service:   serviceCounter,
			transport: transportStdio,
			binary:    rustBinary("counter"),
			env:       func(ports) []string { return nil },
			gaps:      []string{gapNoStructuredContent},
		},
		{
			// The C++ binary always starts its gRPC server and reaches it
			// through a channel the generated adapter points at
			// localhost:50051, which no environment variable moves. Both C++
			// targets therefore take the default port and, like every other
			// target, run serially.
			name:          "cpp/todo/stdio",
			lang:          langCpp,
			service:       serviceTodo,
			transport:     transportStdio,
			binary:        cppBinary,
			fixedGRPCPort: true,
			env:           func(ports) []string { return []string{"MCP_TRANSPORT=stdio"} },
			gaps: []string{
				gapNoStructuredContent,
				gapNoPrompts,
				gapNoResources,
				gapNoCompletion,
				gapNoElicitation,
				gapErrorsReportedAsSuccess,
			},
		},
		{
			name:          "cpp/todo/streamable-http",
			lang:          langCpp,
			service:       serviceTodo,
			transport:     transportHTTP,
			binary:        cppBinary,
			fixedGRPCPort: true,
			env: func(p ports) []string {
				return []string{
					"MCP_TRANSPORT=http",
					"MCP_HOST=127.0.0.1",
					fmt.Sprintf("MCP_PORT=%d", p.mcp),
					"MCP_BASE_PATH=" + todoBasePath,
				}
			},
			endpoint: httpEndpoint(todoBasePath),
			gaps: []string{
				gapNoStructuredContent,
				gapNoPrompts,
				gapNoResources,
				gapNoCompletion,
				gapNoElicitation,
				gapErrorsReportedAsSuccess,
			},
		},
	}
}

// requiredLanguages returns the languages whose absence is a failure rather
// than a skip, read from MCP_CONFORMANCE_REQUIRE (e.g. "rust,cpp").
//
// Skipping by default keeps `go test ./...` useful for someone who has not
// built the Rust and C++ servers. But a skip in CI is indistinguishable from a
// pass in the summary, so each CI job names the language it just built and the
// suite refuses to quietly do nothing.
func requiredLanguages() map[string]bool {
	req := map[string]bool{}
	for _, lang := range strings.Split(os.Getenv("MCP_CONFORMANCE_REQUIRE"), ",") {
		if lang = strings.TrimSpace(lang); lang != "" {
			req[lang] = true
		}
	}
	return req
}

// selectedLanguages returns the languages to run, read from
// MCP_CONFORMANCE_LANGS; empty means all of them.
func selectedLanguages() map[string]bool {
	sel := map[string]bool{}
	for _, lang := range strings.Split(os.Getenv("MCP_CONFORMANCE_LANGS"), ",") {
		if lang = strings.TrimSpace(lang); lang != "" {
			sel[lang] = true
		}
	}
	return sel
}
