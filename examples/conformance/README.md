# MCP Conformance Suite

One MCP client, driving every example server in this repository, over a real
transport, in a separate process.

## Why this exists

The examples used to be covered two ways, and both had the same blind spot.

CI compiled them. A C++ server that starts, completes the MCP handshake, and
advertises **zero tools** compiles and links perfectly — code that is absent has
no build error. That is exactly what `examples/cpp` shipped for a while.

The Go examples also have in-process tests next to them, which register the
generated handlers on an in-memory server and talk to it over an in-memory pipe.
Those are worth keeping — they are fast and they pin the generated Go — but they
never start the binary, so they cannot see which transport it actually serves,
what it advertises after start-up, or whether a handler reaches the gRPC backend
behind it.

This suite closes that gap by treating each server as a black box. It spawns the
shipped binary, speaks MCP to it the way Claude Desktop or MCP Inspector would,
and asserts only on protocol responses. Nothing here imports the generated
packages — which is what lets one suite hold Go, Rust and C++ to a single
contract.

## Running it

The Go example servers are built by the suite itself. The others must be built
first:

```bash
cd examples/rust && cargo build --bins     # rust targets
cd examples/cpp  && make                   # cpp target
```

Then, from `examples/`:

```bash
go test ./conformance/ -v                                   # everything that is built
MCP_CONFORMANCE_LANGS=rust go test ./conformance/ -v        # one language
go test ./conformance/ -run 'TestConformance/cpp' -v        # one target
```

A language whose servers are not built is **skipped**, so `go test ./...` stays
useful without a Rust or C++ toolchain.

| Variable | Effect |
| --- | --- |
| `MCP_CONFORMANCE_LANGS` | Comma-separated languages to run (`go`, `rust`, `cpp`). Empty runs all. |
| `MCP_CONFORMANCE_REQUIRE` | Comma-separated languages whose absence is a **failure** instead of a skip. Each CI job sets this to the language it just built, because a skip reads as a pass in a CI summary. |
| `MCP_CONFORMANCE_RUST_STDIO`<br>`MCP_CONFORMANCE_RUST_HTTP`<br>`MCP_CONFORMANCE_RUST_COUNTER`<br>`MCP_CONFORMANCE_CPP_SERVER` | Point at a binary somewhere other than the conventional in-tree path. |

## What is covered

| | Go | Rust | C++ |
| --- | --- | --- | --- |
| stdio transport | ✅ | ✅ | ✅ |
| streamable-HTTP transport | ✅ | ✅ | ✅ |
| TodoService (CRUD, 5 tools) | ✅ | ✅ | ✅ |
| CounterService (streaming progress) | ✅ | ✅ | — |

Per target, the suite checks:

- **initialize** — server info, protocol version, capabilities.
- **tools/list** — the exact tool set, each tool's description, and the
  `required` array of its `inputSchema` against what the proto declares.
- **tools/call** — a create → get → list → update → get → delete → list round
  trip, reading each write back rather than trusting the mutation's own reply.
- **error reporting** — a call against a missing resource must surface as an MCP
  failure, by either an `isError` result or a JSON-RPC error.
- **prompts, resources, completion** — the `mcp.prompt` options, the app
  resource and resource template, and `completion/complete` over the enum the
  proto declares.
- **elicitation** — tools that declare `mcp.elicitation` must ask before acting;
  the client counts the requests it received.
- **progress** — a streaming tool called with a `progressToken` must report
  advancing progress out of band and deliver its final result.

## Known gaps

The three generators do not produce identical surfaces. Rather than weaken the
assertions to the lowest common denominator, each divergence is declared as a
named gap on the target in `targets_test.go`, with a comment on the constant
explaining it. Assertions are strict by default; a target is excused from one
only by naming it.

| Gap | Who | What |
| --- | --- | --- |
| `structured-content` | Rust, C++ | Tools advertise an `outputSchema` but return text content only, so the result does not keep the contract `tools/list` promised. Go returns both. |
| `prompts`, `resources`, `completion` | C++ | The C++ generator emits tools only. |
| `elicitation` | C++ | `mcp.elicitation` is not emitted, so the C++ server's destructive tools run without confirmation. |
| `http-elicitation-hangs` | Rust (HTTP only) | Elicitation never completes over rmcp's streamable-HTTP, so every mutating todo tool hangs until the client's deadline. The same server completes it over stdio, and the server is observably writing the `elicitation/create` request onto the POST response's SSE stream — the round trip just never closes. Upstream (rmcp or the Go SDK's streamable client), not a generator bug. |
| `http-error-closes-session` | Rust (HTTP only) | A tool error comes back over rmcp's streamable-HTTP as an HTTP 400, which the SDK client treats as a transport failure and closes the session on. Over stdio the same error is an ordinary JSON-RPC error and the session survives. |
| `errors-reported-as-success` | C++ | A failed RPC returns as a *successful* result whose body is `{"error": "..."}` — no `isError`, no JSON-RPC error — so a client is told the call worked and handed an object that does not match the declared `outputSchema`. Structural: the generated adapter passes a JSON string across the cxx boundary with no channel for failure. |

The two HTTP gaps are why the todo subtests run in the order they do: anything
that can leave the session unusable runs last, so one finding does not bury
itself under a cascade of unrelated red.

**Closing a gap is done by deleting its entry and watching the suite go green** —
the list is a to-do, not a specification.

Also not covered: the legacy SSE transport (`sse` entrypoints), and the
`examples/mime` gallery, which has its own tests.

## Adding a target

Add an entry to `targets()` in `targets_test.go`: how to find the binary, the
environment for one run, and which service it serves. The behavioural suites are
picked by the `service` field, so a new server that speaks TodoService is held to
the existing contract without writing any new assertions.
