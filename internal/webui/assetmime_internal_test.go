package webui

import "testing"

// pngBytes/gifBytes are the shortest byte sequences http.DetectContentType
// recognises as those types — enough to exercise the sniff cross-check
// without carrying real image fixtures.
var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\n")
	gifBytes  = []byte("GIF89a")
	jpegBytes = []byte("\xff\xd8\xff")
	htmlBytes = []byte("<!DOCTYPE html><html><body>hi</body></html>")
)

func TestInlineType(t *testing.T) {
	tests := []struct {
		name     string
		asset    string
		data     []byte
		wantType string
		wantOK   bool
	}{
		{name: "png", asset: "logo.png", data: pngBytes, wantType: typePNG, wantOK: true},
		{name: "gif", asset: "spin.gif", data: gifBytes, wantType: typeGIF, wantOK: true},
		{name: "jpeg", asset: "photo.jpg", data: jpegBytes, wantType: typeJPEG, wantOK: true},
		{name: "jpeg via .jpeg", asset: "photo.jpeg", data: jpegBytes, wantType: typeJPEG, wantOK: true},
		{name: "uppercase extension", asset: "LOGO.PNG", data: pngBytes, wantType: typePNG, wantOK: true},

		// The uploader picks the name, so a mismatch must not be served
		// inline no matter how safe the extension looks.
		{name: "html masquerading as png", asset: "evil.png", data: htmlBytes},
		{name: "png bytes under a gif name", asset: "confused.gif", data: pngBytes},

		{name: "svg is never inline", asset: "logo.svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{name: "plain text", asset: "notes.txt", data: []byte("hello")},
		{name: "no extension", asset: "logo", data: pngBytes},
		{name: "empty data", asset: "logo.png", data: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := inlineType(tt.asset, tt.data)
			if gotOK != tt.wantOK || gotType != tt.wantType {
				t.Errorf("inlineType(%q) = (%q, %v), want (%q, %v)", tt.asset, gotType, gotOK, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestIsInlineExtension(t *testing.T) {
	inline := []string{"logo.png", "photo.JPG", "a.jpeg", "b.gif", "c.webp"}
	for _, name := range inline {
		if !isInlineExtension(name) {
			t.Errorf("isInlineExtension(%q) = false, want true", name)
		}
	}

	notInline := []string{"logo.svg", "notes.txt", "archive.zip", "logo", "logo.png.txt"}
	for _, name := range notInline {
		if isInlineExtension(name) {
			t.Errorf("isInlineExtension(%q) = true, want false", name)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.in); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
