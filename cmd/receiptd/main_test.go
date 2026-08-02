package main

import (
	"strings"
	"testing"
)

// A test binary is built without .goreleaser.yml's -ldflags, so the
// placeholder defaults are what versionLine sees here.
func TestVersionLineFormat(t *testing.T) {
	got := versionLine()

	if want := "receiptd dev (commit none, built unknown)"; got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("versionLine() = %q, want a single line", got)
	}
}
