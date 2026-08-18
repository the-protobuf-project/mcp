package conformance

import (
	"context"
	"fmt"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestConformance runs every behavioural suite against every example server.
//
// Subtests are not parallel. Two of the example servers reach their backend on
// a gRPC port that no environment variable moves, so overlapping runs would
// contend for it; and a failure here should name one server, not a race between
// two.
func TestConformance(t *testing.T) {
	required := requiredLanguages()
	selected := selectedLanguages()

	ran := map[string]bool{}

	for _, tg := range targets() {
		if len(selected) > 0 && !selected[tg.lang] {
			continue
		}
		t.Run(tg.name, func(t *testing.T) {
			bin, missing := tg.binary(t)
			if missing != "" {
				if required[tg.lang] {
					t.Fatalf("MCP_CONFORMANCE_REQUIRE lists %q, but its server is not built: %s", tg.lang, missing)
				}
				t.Skipf("%s server unavailable: %s", tg.lang, missing)
			}
			ran[tg.lang] = true
			runTarget(t, tg, bin)
		})
	}

	// A skipped subtest reads as a pass in most CI summaries, so a job that
	// asked for a language and then ran none of it fails loudly here.
	for lang := range required {
		if !ran[lang] {
			t.Errorf("MCP_CONFORMANCE_REQUIRE lists %q but no %s target ran", lang, lang)
		}
	}
}

// runTarget connects to one target and dispatches to the suite for its service.
func runTarget(t *testing.T, tg target, bin string) {
	ctx := context.Background()
	deps := newSessionDeps()

	p := ports{}
	if !tg.fixedGRPCPort {
		p.grpc = freePort(t)
	}

	var session *mcpsdk.ClientSession
	switch tg.transport {
	case transportStdio:
		session = connectStdio(t, ctx, bin, tg.env(p), deps)
	case transportHTTP:
		p.mcp = freePort(t)
		session = connectHTTP(t, ctx, bin, tg.env(p), tg.endpoint(p), p.mcp, deps)
	default:
		t.Fatalf("unknown transport %q", tg.transport)
	}

	assertInitialized(t, session)

	switch tg.service {
	case serviceTodo:
		runTodoSuite(t, tg, session, deps.elicits)
	case serviceCounter:
		runCounterSuite(t, tg, session, deps.progress)
	default:
		t.Fatalf("unknown service %q", tg.service)
	}
}

// assertInitialized checks the handshake result the session was built from.
//
// Reaching this point already proves initialize succeeded, so what is left to
// check is that the server identified itself and negotiated a protocol version
// -- an empty serverInfo is what a client shows the user in a server list.
func assertInitialized(t *testing.T, session *mcpsdk.ClientSession) {
	t.Helper()
	res := session.InitializeResult()
	if res == nil {
		t.Fatal("no initialize result on an established session")
	}
	if res.ServerInfo == nil || strings.TrimSpace(res.ServerInfo.Name) == "" {
		t.Error("initialize returned no serverInfo.name")
	}
	if strings.TrimSpace(res.ProtocolVersion) == "" {
		t.Error("initialize returned no protocolVersion")
	}
	if res.Capabilities == nil {
		t.Error("initialize returned no capabilities")
	}
	t.Logf("connected: %s %s (MCP %s)",
		serverName(res), serverVersion(res), res.ProtocolVersion)
}

func serverName(res *mcpsdk.InitializeResult) string {
	if res.ServerInfo == nil {
		return "<unknown>"
	}
	return res.ServerInfo.Name
}

func serverVersion(res *mcpsdk.InitializeResult) string {
	if res.ServerInfo == nil || res.ServerInfo.Version == "" {
		return "<no version>"
	}
	return fmt.Sprintf("v%s", res.ServerInfo.Version)
}
