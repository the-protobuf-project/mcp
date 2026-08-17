# Media types: the gallery example

`GalleryService` publishes the same kind of thing — an asset — under six IANA
media types, so an MCP client has something concrete to render for each one. It
is the worked example for the "Media types" section of
[`protobuf/README.md`](../../protobuf/README.md).

| Asset | `mime_type` | Travels as | Client renders |
| --- | --- | --- | --- |
| `overview` | `text/markdown` | text | prose |
| `report` | `text/html` | text | a document |
| `manifest` | `application/json` | text | structured data |
| `downloads` | `text/csv` | text | a table |
| `logo` | `image/png` | base64 bytes | an inline image |
| `spec` | `application/pdf` | base64 bytes | a download |

Plus a resource *template* (`gallery://assets/{asset}`) declared as
`application/octet-stream`, and the MCP App UI resource the `app` block
generates, which is `text/html`.

## What it demonstrates

**The media type is the whole contract.** `content/logo.png` and
`content/spec.pdf` are both opaque bytes; the only reason a client shows one
inline and offers the other as a download is `mime_type`. `readAsset` in
[`main.go`](main.go) picks `ResourceContents.Text` or `.Blob` from
`isTextual(mimeType)` and nothing else.

**Metadata stays in the proto.** The example supplies only content, via
`mcp.WithResourceHandler(uri, handler)`. Title, media type, size, annotations
and icons are never restated in Go, so there is one source of truth for what a
resource *is*.

**It is a free-form string, not an enum.** `text/csv` is in the gallery
specifically because nothing in the schema enumerates it. See
[AIP-143](https://google.aip.dev/143).

**Resource metadata reaches the UI.** The `overview` asset declares `title`,
`size`, `annotations` (audience, priority, `last_modified`) and two `icons` with
`light`/`dark` themes. All of it is emitted into the generated registration and
shows up in `resources/list` — [`gallery_test.go`](gallery_test.go) asserts it.

**The MCP App surface.** The `app` block on the service generates a
`ui://galleryservice/app.html` resource with media type `text/html`, which is
what a client treats as an app UI rather than a document.

## Layout

```
proto/gallery/v1/     the schema — resources, tools, prompt
content/              the bytes served for each asset
gen/go|rust/          generated code (`just generate-mime`)

assets.go             the gallery table and the text-vs-binary rule
impl.go               GalleryServiceMCPServer implementation
main.go               server wiring; supplies content via WithResourceHandler
gallery_test.go       Go validation, incl. a live client over HTTP

rust/                 Rust crate that type-checks and runs the generated handler
```

## Running it

```sh
just check-mime       # regenerate and validate both languages

go run .              # Go server, HTTP transport on :8080
go run . -stdio       # stdio transport
```

**MCP Inspector**: transport *Streamable HTTP*, URL
`http://localhost:8080/gallery/v1/galleryservice/mcp`. The path comes from the
proto — `gallery.v1.GalleryService` lowercased, dots to slashes, plus `/mcp` —
so plain `/mcp` is a 404. `-addr` changes the port, never the path.

Verified end to end against the running server: `resources/list` returns all six
media types plus the app UI resource, and `resources/read` on
`gallery://images/logo.png` returns base64 that decodes byte-identical to
`content/logo.png`.

## The two languages

Each validates the same thing from its own side — that a media type declared in
the proto reaches the client, along with the metadata beside it.

| | Validation |
| --- | --- |
| Go | `go test ./...` — six tests, including a live MCP client over HTTP |
| Rust | `cargo test` — three tests; the generated `resources()` parses into rmcp's types at run time, so this runs it rather than only compiling it |

Go reads the annotations at run time but reaches them through `protobuf/mcppb`,
and Rust's prost drops options entirely, so neither generates the annotation
types alongside the gallery.

## Why it is isolated

The gallery uses schema features — service-level `resources`, resource `title` /
`size` / `annotations` / `icons` — that exist only in this repository until the
next `buf push`. So it carries the `mcp.v1` annotations while the other examples
carry the published `mcp.protobuf` ones, and **the two cannot meet**: they claim
the same extension numbers (51000–51006) on the same descriptor messages.

That collision shows up in two places, which is why the isolation is twofold:

- **buf** — one image cannot compile both: `field number 51000 used more than once`.
- **Go** — one binary cannot link both: `proto: extension number 51000 is already
  registered on message google.protobuf.ServiceOptions`.

Rust is unaffected, since prost never registers the annotations at all.

So the gallery has its own buf module and its own Go module. Both fold back into
the main examples the moment `mcp.v1` is published and every example moves to it
together.
