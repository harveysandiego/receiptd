package webui

import (
	"net/http"
	"path/filepath"
	"strings"
)

// inlineImageTypes is the closed set of types an asset may be served with
// inline. SVG is absent deliberately: it is an image by extension but
// executes script when a browser renders it, and assets are
// attacker-supplied. See
// docs/adr/0029-asset-content-endpoint-inline-type-allowlist.md.
const (
	typePNG  = "image/png"
	typeJPEG = "image/jpeg"
	typeGIF  = "image/gif"
	typeWebP = "image/webp"
)

var inlineImageTypes = map[string]string{
	".png":  typePNG,
	".jpg":  typeJPEG,
	".jpeg": typeJPEG,
	".gif":  typeGIF,
	".webp": typeWebP,
}

// isInlineExtension reports whether name's extension is one the Assets
// page may render in an <img>. It is the listing page's test, which has
// only a name to go on; inlineType is what actually gates serving.
func isInlineExtension(name string) bool {
	_, ok := inlineImageTypes[strings.ToLower(filepath.Ext(name))]
	return ok
}

// inlineType returns the Content-Type data may be served with inline, and
// whether inline is allowed at all. The extension must be in
// inlineImageTypes and the bytes must sniff to the same type: the
// uploader chooses the name, so it is never the sole authority on what a
// file is.
func inlineType(name string, data []byte) (string, bool) {
	declared, ok := inlineImageTypes[strings.ToLower(filepath.Ext(name))]
	if !ok {
		return "", false
	}
	// DetectContentType appends parameters for some types, so compare only
	// the media type.
	sniffed, _, _ := strings.Cut(http.DetectContentType(data), ";")
	if strings.TrimSpace(sniffed) != declared {
		return "", false
	}
	return declared, true
}
