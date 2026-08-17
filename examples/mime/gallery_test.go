package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	galleryv1 "github.com/the-protobuf-project/mcp/examples/mime/gen/go/gallery/v1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

// connect builds the gallery MCP server exactly as main does, serves it over a
// local HTTP transport, and returns a connected client session.
func connect(t *testing.T) (*mcp.ClientSession, context.Context) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	server := mcp.NewMCPServer(&mcp.MCPServerConfig{Name: "gallery-test", Version: "1.0.0"})
	galleryv1.RegisterGalleryServiceMCPHandler(server, &galleryServer{}, contentHandlers()...)

	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil,
	))
	t.Cleanup(ts.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, ctx
}

// Every declared media type must survive into the resource listing, since that
// is the only thing telling a client how to render each asset.
func TestListResourcesCarriesDeclaredMediaTypes(t *testing.T) {
	session, ctx := connect(t)

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	got := make(map[string]*mcp.Resource, len(res.Resources))
	for _, r := range res.Resources {
		got[r.URI] = r
	}

	for _, want := range assets {
		r, ok := got[want.uri]
		if !ok {
			t.Errorf("resource %s missing from listing", want.uri)
			continue
		}
		if r.MIMEType != want.mimeType {
			t.Errorf("%s: MIMEType = %q, want %q", want.uri, r.MIMEType, want.mimeType)
		}
		if r.Title != want.title {
			t.Errorf("%s: Title = %q, want %q", want.uri, r.Title, want.title)
		}
	}
}

// Resource metadata declared in the proto — annotations and icons — has to
// reach the client, not just the media type.
func TestListResourcesCarriesAnnotationsAndIcons(t *testing.T) {
	session, ctx := connect(t)

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var overview *mcp.Resource
	for _, r := range res.Resources {
		if r.URI == "gallery://docs/overview.md" {
			overview = r
		}
	}
	if overview == nil {
		t.Fatal("overview resource missing")
	}
	if overview.Annotations == nil {
		t.Fatal("overview: Annotations = nil, want audience/priority from the proto")
	}
	if got, want := len(overview.Annotations.Audience), 2; got != want {
		t.Errorf("overview: %d audience entries, want %d", got, want)
	}
	if got, want := overview.Annotations.Priority, 1.0; got != want {
		t.Errorf("overview: Priority = %v, want %v", got, want)
	}
	if got, want := len(overview.Icons), 2; got != want {
		t.Errorf("overview: %d icons, want %d (a light and a dark variant)", got, want)
	}
}

// Textual media types must arrive as text and binary ones as a blob; a client
// picks how to present the bytes from the media type alone.
func TestReadResourceSplitsTextAndBinaryByMediaType(t *testing.T) {
	session, ctx := connect(t)

	for _, a := range assets {
		t.Run(a.id, func(t *testing.T) {
			res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: a.uri})
			if err != nil {
				t.Fatalf("ReadResource(%s): %v", a.uri, err)
			}
			if len(res.Contents) != 1 {
				t.Fatalf("got %d contents, want 1", len(res.Contents))
			}
			c := res.Contents[0]
			if c.MIMEType != a.mimeType {
				t.Errorf("MIMEType = %q, want %q", c.MIMEType, a.mimeType)
			}
			if isTextual(a.mimeType) {
				if c.Text == "" {
					t.Error("textual media type returned no text")
				}
				if len(c.Blob) != 0 {
					t.Error("textual media type also returned a blob")
				}
				return
			}
			if len(c.Blob) == 0 {
				t.Error("binary media type returned no blob")
			}
			if c.Text != "" {
				t.Error("binary media type also returned text")
			}
		})
	}
}

// The PNG and PDF must survive the base64 round trip intact, or a client
// renders a broken image.
func TestBinaryAssetsRoundTrip(t *testing.T) {
	session, ctx := connect(t)

	for _, id := range []string{"logo", "spec"} {
		a, ok := assetByID(id)
		if !ok {
			t.Fatalf("asset %q missing", id)
		}
		res, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: a.uri})
		if err != nil {
			t.Fatalf("ReadResource(%s): %v", a.uri, err)
		}
		if got := res.Contents[0].Blob; string(got) != string(a.blob) {
			t.Errorf("%s: blob round trip changed the bytes (%d in, %d out)", id, len(a.blob), len(got))
		}
	}
}

// The UI resource the MCP App generates is HTML, which is what makes a client
// render it as an app surface rather than dumping markup.
func TestAppResourceIsHTML(t *testing.T) {
	session, ctx := connect(t)

	res, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	appURI := mcp.AppResourceURI("GalleryService")
	for _, r := range res.Resources {
		if r.URI == appURI {
			if r.MIMEType != "text/html" {
				t.Errorf("app resource MIMEType = %q, want text/html", r.MIMEType)
			}
			return
		}
	}
	t.Errorf("app resource %s missing from listing", appURI)
}

// The tool filter is the media type, so an unknown one must fail loudly rather
// than quietly returning nothing.
func TestListAssetsFiltersByMediaType(t *testing.T) {
	srv := &galleryServer{}

	got, err := srv.ListAssets(context.Background(), &galleryv1.ListAssetsRequest{MimeType: "text/markdown"})
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(got.GetAssets()) != 1 || got.GetAssets()[0].GetId() != "overview" {
		t.Errorf("filtering on text/markdown returned %v, want just overview", got.GetAssets())
	}

	if _, err := srv.ListAssets(context.Background(), &galleryv1.ListAssetsRequest{MimeType: "audio/flac"}); err == nil {
		t.Error("filtering on an absent media type: got nil error, want one")
	}
}
