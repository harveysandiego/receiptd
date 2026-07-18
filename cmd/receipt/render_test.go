package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

const validReceiptJSON = `{
	"version": 1,
	"elements": [
		{"type": "heading", "content": "Shopping List"},
		{"type": "text", "content": "Milk"},
		{"type": "spacer", "height": 10}
	]
}`

// writeInput writes content to a new file under t.TempDir() named
// "receipt.json" and returns its path.
func writeInput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v, want nil", path, err)
	}
	return path
}

func execRender(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	return cmd.Execute()
}

func TestRender_ValidReceipt_ProducesDecodablePNG(t *testing.T) {
	in := writeInput(t, validReceiptJSON)
	out := filepath.Join(t.TempDir(), "preview.png")

	if err := execRender(t, "render", in, "--out", out); err != nil {
		t.Fatalf("execute render error = %v, want nil", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v, want nil", out, err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v, want nil", err)
	}
	if b := img.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("decoded image bounds = %v, want non-empty", b)
	}
}

func TestRender_MissingOutFlag_Fails(t *testing.T) {
	in := writeInput(t, validReceiptJSON)

	if err := execRender(t, "render", in); err == nil {
		t.Fatal("execute render error = nil, want non-nil (missing --out)")
	}
}

func TestRender_InputFailure_FailsWithoutCreatingOutput(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"malformed element", `{"version": 1, "elements": [{"type": "text", "content": 123}]}`},
		{"failed validation", `{"version": 1, "elements": [{"type": "text", "content": ""}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := writeInput(t, tt.json)
			out := filepath.Join(t.TempDir(), "preview.png")

			if err := execRender(t, "render", in, "--out", out); err == nil {
				t.Fatal("execute render error = nil, want non-nil")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Errorf("os.Stat(%q) error = %v, want IsNotExist", out, err)
			}
		})
	}
}

func TestRender_MissingInputFile_FailsCleanly(t *testing.T) {
	out := filepath.Join(t.TempDir(), "preview.png")
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")

	if err := execRender(t, "render", missing, "--out", out); err == nil {
		t.Fatal("execute render error = nil, want non-nil (missing input file)")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("os.Stat(%q) error = %v, want IsNotExist", out, err)
	}
}
