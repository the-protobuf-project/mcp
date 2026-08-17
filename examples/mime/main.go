// Command gallery runs an MCP server that publishes the same kind of asset
// under six IANA media types, so a client has something concrete to render for
// each. See README.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	galleryv1 "github.com/the-protobuf-project/mcp/examples/mime/gen/go/gallery/v1"
	"github.com/the-protobuf-project/runtime-go/agents/mcp"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address for the HTTP transport")
	stdio := flag.Bool("stdio", false, "serve over stdio instead of HTTP")
	flag.Parse()

	cfg := &mcp.MCPServerConfig{
		Name:              "gallery-mcp",
		Version:           "1.0.0",
		Addr:              *addr,
		GeneratedBasePath: galleryv1.GalleryServiceMCPDefaultBasePath,
	}
	if *stdio {
		cfg.Transports = []mcp.Transport{mcp.TransportStreamableHTTP}
	}

	srv := &galleryServer{}

	err := mcp.StartServer(context.Background(), cfg, func(s *mcp.Server) {
		// Registers the tools, the prompt, the MCP App UI resource, and the
		// seven resources declared on GalleryService — each with the media
		// type, title, size, annotations and icons from the proto.
		galleryv1.RegisterGalleryServiceMCPHandler(s, srv, contentHandlers()...)
	})
	if err != nil {
		log.Fatalf("gallery MCP server: %v", err)
	}
}

// contentHandlers serves each declared resource's real bytes in place of the
// generated placeholder.
//
// Only the content is overridden: the media type, title, size, annotations and
// icons still come from the proto, so there is one source of truth for what a
// resource *is* and Go supplies only what it holds.
func contentHandlers() []mcp.Option {
	opts := make([]mcp.Option, 0, len(assets))
	for _, a := range assets {
		opts = append(opts, mcp.WithResourceHandler(a.uri, readAsset))
	}
	return opts
}

func readAsset(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	a, ok := assetByURI(req.Params.URI)
	if !ok {
		return nil, fmt.Errorf("no asset served at %s", req.Params.URI)
	}
	contents := &mcp.ResourceContents{
		URI:      a.uri,
		MIMEType: a.mimeType,
	}
	if isTextual(a.mimeType) {
		contents.Text = a.text
	} else {
		contents.Blob = a.blob
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{contents}}, nil
}
