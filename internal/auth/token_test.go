package auth_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/auth"
	"github.com/harveysandiego/receiptd/internal/config"
)

func writeTokenFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	return path
}

// unsetEnvToken guarantees auth.EnvToken is absent for the duration of the
// test, regardless of the ambient environment, and restores its original
// presence/value afterwards.
func unsetEnvToken(t *testing.T) {
	t.Helper()
	orig, wasSet := os.LookupEnv(auth.EnvToken)
	if err := os.Unsetenv(auth.EnvToken); err != nil {
		t.Fatalf("os.Unsetenv: %v", err)
	}
	t.Cleanup(func() {
		if wasSet {
			if err := os.Setenv(auth.EnvToken, orig); err != nil {
				t.Fatalf("os.Setenv (restore): %v", err)
			}
		} else {
			if err := os.Unsetenv(auth.EnvToken); err != nil {
				t.Fatalf("os.Unsetenv (restore): %v", err)
			}
		}
	})
}

func TestResolveToken_Disabled(t *testing.T) {
	t.Setenv(auth.EnvToken, "should-be-ignored")

	tok, err := auth.ResolveToken(config.AuthConfig{Enabled: false})
	if err != nil {
		t.Fatalf("ResolveToken: unexpected error: %v", err)
	}
	if tok != "" {
		t.Errorf("ResolveToken = %q, want empty", tok)
	}
}

func TestResolveToken_EnvOverridesTokenFile(t *testing.T) {
	path := writeTokenFile(t, "file-token\n")
	t.Setenv(auth.EnvToken, "env-token")

	tok, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err != nil {
		t.Fatalf("ResolveToken: unexpected error: %v", err)
	}
	if got, want := tok, "env-token"; got != want {
		t.Errorf("ResolveToken = %q, want %q", got, want)
	}
}

func TestResolveToken_TokenFileFallback(t *testing.T) {
	unsetEnvToken(t)
	path := writeTokenFile(t, "file-token\n")

	tok, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err != nil {
		t.Fatalf("ResolveToken: unexpected error: %v", err)
	}
	if got, want := tok, "file-token"; got != want {
		t.Errorf("ResolveToken = %q, want %q", got, want)
	}
}

func TestResolveToken_TokenFileTrimsTrailingWhitespaceOnly(t *testing.T) {
	unsetEnvToken(t)
	path := writeTokenFile(t, "  file-token  \r\n")

	tok, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err != nil {
		t.Fatalf("ResolveToken: unexpected error: %v", err)
	}
	if got, want := tok, "  file-token"; got != want {
		t.Errorf("ResolveToken = %q, want %q (leading whitespace preserved, trailing removed)", got, want)
	}
}

func TestResolveToken_MissingTokenFile(t *testing.T) {
	unsetEnvToken(t)
	path := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err == nil {
		t.Fatal("ResolveToken: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindNotFound) {
		t.Errorf("ResolveToken: err = %v, want apperr.KindNotFound", err)
	}
}

func TestResolveToken_TokenFileReadError(t *testing.T) {
	unsetEnvToken(t)
	// A directory is a portable fixture for a read failure other than
	// "not found" (os.ReadFile fails on it with EISDIR-equivalent).
	dir := t.TempDir()

	_, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: dir})
	if err == nil {
		t.Fatal("ResolveToken: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Errorf("ResolveToken: err = %v, want apperr.KindPermanent", err)
	}
}

func TestResolveToken_NoTokenSourceConfigured(t *testing.T) {
	unsetEnvToken(t)

	_, err := auth.ResolveToken(config.AuthConfig{Enabled: true})
	if err == nil {
		t.Fatal("ResolveToken: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindValidation) {
		t.Errorf("ResolveToken: err = %v, want apperr.KindValidation", err)
	}
}

func TestResolveToken_EmptyTokenFile(t *testing.T) {
	unsetEnvToken(t)
	path := writeTokenFile(t, "   \n")

	_, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err == nil {
		t.Fatal("ResolveToken: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindValidation) {
		t.Errorf("ResolveToken: err = %v, want apperr.KindValidation", err)
	}
}

func TestResolveToken_EmptyEnvOverride(t *testing.T) {
	path := writeTokenFile(t, "file-token\n")
	t.Setenv(auth.EnvToken, "")

	_, err := auth.ResolveToken(config.AuthConfig{Enabled: true, TokenFile: path})
	if err == nil {
		t.Fatal("ResolveToken: expected error, got nil")
	}
	if !apperr.Is(err, apperr.KindValidation) {
		t.Errorf("ResolveToken: err = %v, want apperr.KindValidation", err)
	}
}
