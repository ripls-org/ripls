package secretsflag

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Resolve returns the credential value: if path is non-empty, reads it from
// the file; if value is non-empty, returns it directly. Setting both is an
// error; setting neither returns the empty string and lets the caller decide
// whether that constitutes a missing-required-flag failure.
//
// Trailing whitespace — spaces, tabs, carriage returns, and one or more
// newlines — is stripped from the returned value, whether it came from a file
// or a flag argument. Secret Manager uploads frequently pick up an unintended
// trailing newline (or CRLF), which silently corrupts exact-match credentials
// such as API keys, signing secrets, and OAuth client IDs. Leading whitespace
// and embedded newlines (e.g. multi-line PEM payloads) are preserved.
//
// flagName is the bare flag name without the -file suffix (e.g.
// "openai-api-key"); it is used only to compose error messages and is never
// the source of the returned secret.
func Resolve(flagName, value, path string) (string, error) {
	if value != "" && path != "" {
		return "", fmt.Errorf("flags -%s and -%s-file are mutually exclusive", flagName, flagName)
	}
	if path == "" {
		return trimSecret(value), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read -%s-file=%s: %w", flagName, path, err)
	}
	return trimSecret(string(data)), nil
}

// trimSecret strips trailing whitespace — spaces, tabs, carriage returns, and
// newlines — from the end of a secret value. Only the trailing run is removed,
// so leading whitespace and newlines embedded in multi-line payloads (PEM
// keys) survive intact.
func trimSecret(s string) string {
	return strings.TrimRight(s, " \t\r\n")
}

// InjectPostgresPassword rewrites a postgres connection URL so the password
// comes from a file rather than from the URL itself. If passwordFilePath is
// empty, dbURL is returned unchanged.
//
// The URL must include a username; an existing password in the URL is
// rejected as ambiguous so operators never end up with two passwords floating
// around at once.
func InjectPostgresPassword(dbURL, passwordFilePath string) (string, error) {
	if passwordFilePath == "" {
		return dbURL, nil
	}
	password, err := Resolve("db-password", "", passwordFilePath)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(dbURL)
	if err != nil {
		return "", fmt.Errorf("parse -db URL: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return "", errors.New("-db URL must include a username when -db-password-file is set")
	}
	if _, hasPwd := u.User.Password(); hasPwd {
		return "", errors.New("-db URL embeds a password and -db-password-file is also set; choose one")
	}
	u.User = url.UserPassword(u.User.Username(), password)
	return u.String(), nil
}
