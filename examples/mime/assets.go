package main

import (
	_ "embed"
	"sort"
)

// asset is one item in the gallery, paired with the bytes served for it.
//
// The media type is the only thing that tells an MCP client how to present the
// content: the same []byte is prose, markup, a table, or an image depending on
// what this field says. Nothing in the schema constrains the value — it is an
// IANA media type string, and the registry grows without the schema changing.
type asset struct {
	id       string
	title    string
	uri      string
	mimeType string
	// text is set for textual media types; blob for binary ones. Exactly one is
	// populated, decided by isTextual(mimeType).
	text string
	blob []byte
}

//go:embed content/overview.md
var overviewMarkdown string

//go:embed content/report.html
var reportHTML string

//go:embed content/manifest.json
var manifestJSON string

//go:embed content/downloads.csv
var downloadsCSV string

//go:embed content/logo.png
var logoPNG []byte

//go:embed content/spec.pdf
var specPDF []byte

// assets is the gallery, in the same order the resources are declared on
// GalleryService in gallery_service.proto.
var assets = []asset{
	{
		id:       "overview",
		title:    "Gallery overview",
		uri:      "gallery://docs/overview.md",
		mimeType: "text/markdown",
		text:     overviewMarkdown,
	},
	{
		id:       "report",
		title:    "Quarterly report",
		uri:      "gallery://docs/report.html",
		mimeType: "text/html",
		text:     reportHTML,
	},
	{
		id:       "manifest",
		title:    "Asset manifest",
		uri:      "gallery://data/manifest.json",
		mimeType: "application/json",
		text:     manifestJSON,
	},
	{
		id:       "downloads",
		title:    "Download counts",
		uri:      "gallery://data/downloads.csv",
		mimeType: "text/csv",
		text:     downloadsCSV,
	},
	{
		id:       "logo",
		title:    "The Protobuf Project logo",
		uri:      "gallery://images/logo.png",
		mimeType: "image/png",
		blob:     logoPNG,
	},
	{
		id:       "spec",
		title:    "Format specification",
		uri:      "gallery://docs/spec.pdf",
		mimeType: "application/pdf",
		blob:     specPDF,
	},
}

// isTextual reports whether content of this media type travels as `text` rather
// than base64 `blob`. Anything under text/* is textual, as are the structured
// types that are textual despite their application/* prefix.
func isTextual(mimeType string) bool {
	if len(mimeType) >= 5 && mimeType[:5] == "text/" {
		return true
	}
	switch mimeType {
	case "application/json", "application/xml", "image/svg+xml":
		return true
	default:
		return false
	}
}

// assetByID returns the asset with the given id.
func assetByID(id string) (asset, bool) {
	for _, a := range assets {
		if a.id == id {
			return a, true
		}
	}
	return asset{}, false
}

// assetByURI returns the asset served at the given URI.
func assetByURI(uri string) (asset, bool) {
	for _, a := range assets {
		if a.uri == uri {
			return a, true
		}
	}
	return asset{}, false
}

// sizeBytes is the length of whichever representation this asset uses.
func (a asset) sizeBytes() int64 {
	if isTextual(a.mimeType) {
		return int64(len(a.text))
	}
	return int64(len(a.blob))
}

// mimeTypes returns the distinct media types in the gallery, sorted.
func mimeTypes() []string {
	seen := make(map[string]bool, len(assets))
	var out []string
	for _, a := range assets {
		if !seen[a.mimeType] {
			seen[a.mimeType] = true
			out = append(out, a.mimeType)
		}
	}
	sort.Strings(out)
	return out
}
