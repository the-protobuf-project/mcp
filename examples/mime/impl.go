package main

import (
	"context"
	"fmt"

	galleryv1 "github.com/the-protobuf-project/mcp/examples/mime/gen/go/gallery/v1"
)

var _ galleryv1.GalleryServiceMCPServer = (*galleryServer)(nil)

type galleryServer struct{}

// ListAssets returns the gallery, optionally narrowed to one media type.
func (s *galleryServer) ListAssets(_ context.Context, req *galleryv1.ListAssetsRequest) (*galleryv1.ListAssetsResponse, error) {
	want := req.GetMimeType()
	resp := &galleryv1.ListAssetsResponse{}
	for _, a := range assets {
		if want != "" && a.mimeType != want {
			continue
		}
		resp.Assets = append(resp.Assets, &galleryv1.Asset{
			Id:        a.id,
			Title:     a.title,
			MimeType:  a.mimeType,
			Uri:       a.uri,
			SizeBytes: a.sizeBytes(),
		})
	}
	if len(resp.Assets) == 0 && want != "" {
		return nil, fmt.Errorf("no assets with media type %q; gallery holds %v", want, mimeTypes())
	}
	return resp, nil
}

// GetAsset returns one asset's content. Which field carries it — text or data —
// follows from the media type, and the caller is told the type either way.
func (s *galleryServer) GetAsset(_ context.Context, req *galleryv1.GetAssetRequest) (*galleryv1.GetAssetResponse, error) {
	a, ok := assetByID(req.GetId())
	if !ok {
		return nil, fmt.Errorf("asset %q not found", req.GetId())
	}
	resp := &galleryv1.GetAssetResponse{
		Asset: &galleryv1.Asset{
			Id:        a.id,
			Title:     a.title,
			MimeType:  a.mimeType,
			Uri:       a.uri,
			SizeBytes: a.sizeBytes(),
		},
	}
	if isTextual(a.mimeType) {
		resp.Text = a.text
	} else {
		resp.Data = a.blob
	}
	return resp, nil
}
