package auth

import (
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/config"
)

// EnvToken is the environment variable that overrides auth.token_file
// (docs/ARCHITECTURE.md §7).
const EnvToken = "RECEIPTD_AUTH_TOKEN"

// ResolveToken resolves the shared token configured by cfg. When auth is
// disabled, it returns "", nil without requiring or reading a token
// source. When enabled, EnvToken is authoritative if set; otherwise
// cfg.TokenFile is read, with only its trailing newline/whitespace
// trimmed (leading bytes are preserved verbatim). A missing token source
// or an empty effective token is a validation error.
func ResolveToken(cfg config.AuthConfig) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}

	if tok, ok := os.LookupEnv(EnvToken); ok {
		if tok == "" {
			return "", apperr.Wrap(apperr.KindValidation, "auth.ResolveToken", errors.New(EnvToken+" is set but empty"))
		}
		return tok, nil
	}

	if cfg.TokenFile == "" {
		return "", apperr.Wrap(apperr.KindValidation, "auth.ResolveToken", errors.New("auth enabled but no token source configured (RECEIPTD_AUTH_TOKEN or auth.token_file)"))
	}

	data, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", apperr.Wrap(apperr.KindNotFound, "auth.ResolveToken", err)
		}
		return "", apperr.Wrap(apperr.KindPermanent, "auth.ResolveToken", err)
	}

	tok := strings.TrimRight(string(data), " \t\r\n")
	if tok == "" {
		return "", apperr.Wrap(apperr.KindValidation, "auth.ResolveToken", errors.New("token file is empty"))
	}
	return tok, nil
}
