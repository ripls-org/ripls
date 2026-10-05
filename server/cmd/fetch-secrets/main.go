// Command fetch-secrets writes Secret Manager secrets into a directory, one file
// per secret, for the container entrypoint (server/entrypoint.sh) to hand to the
// server as -<name>-file flags.
//
// It authenticates with Application Default Credentials, so the same binary
// works on Cloud Run (the metadata server's token for the runtime service
// account) and on any other host (a service-account key named by
// GOOGLE_APPLICATION_CREDENTIALS). The server itself never talks to a secret
// store; see docs/secrets.md.
//
// Each positional argument names a required secret; -optional names one whose
// absence is tolerated. Either form is FILE or FILE=SECRET, where FILE is the
// file written inside -dir (and so the server flag it becomes) and SECRET is the
// Secret Manager name, defaulting to FILE.
//
// Usage:
//
//	fetch-secrets -project=my-project -dir=/tmp/secrets \
//	  -optional=twilio-auth-token \
//	  jwt-signing-secret \
//	  github-app-private-key=github-app-private-key-base64
//
// A required secret that cannot be read, or reads as empty, fails the run. An
// optional one that does not exist, is not readable by this identity, or has no
// enabled version is written as an empty file and logged, so the server leaves
// that feature unconfigured. Any other failure — network, quota, a server error —
// fails the run whichever kind the secret is: a transient error must not quietly
// disable a feature for the life of the process.
//
// Values are never logged. Trailing whitespace is stripped from each value, as
// the server does when it reads the file (server/secretsflag).
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

const (
	// apiEndpoint is the Secret Manager REST API.
	apiEndpoint        = "https://secretmanager.googleapis.com"
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

	// maxResponseBytes bounds a single access response. Secret Manager caps a
	// payload at 64 KiB, which is under 90 KiB once base64-encoded in JSON.
	maxResponseBytes = 1 << 20
)

var (
	// fileNamePattern is the shape of a server flag stem: the file becomes
	// -<name>-file, so anything else could never be a valid flag.
	fileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// secretNamePattern is Secret Manager's own constraint on secret IDs.
	secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
)

// errSecretAbsent marks a secret that does not exist, is not readable by this
// identity, or has no enabled version — the cases an optional secret tolerates.
var errSecretAbsent = errors.New("secret absent")

// secretSpec is one secret to fetch and the file it is written to.
type secretSpec struct {
	file     string
	secret   string
	optional bool
}

// config is the parsed command line.
type config struct {
	project string
	dir     string
	timeout time.Duration
	specs   []secretSpec
}

// specList collects repeated -optional flags.
type specList []string

func (l *specList) String() string { return strings.Join(*l, ",") }

func (l *specList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func main() {
	cfg, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "fetch-secrets: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	client, err := google.DefaultClient(ctx, cloudPlatformScope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch-secrets: no Application Default Credentials: %v\n"+
			"  On Cloud Run these come from the runtime service account. Elsewhere, set\n"+
			"  GOOGLE_APPLICATION_CREDENTIALS to a service-account key file.\n", err)
		os.Exit(1)
	}

	f := &fetcher{client: client, endpoint: apiEndpoint, project: cfg.project, log: os.Stderr}
	if err := f.fetchAll(ctx, cfg.dir, cfg.specs); err != nil {
		fmt.Fprintf(os.Stderr, "fetch-secrets: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs parses and validates the command line.
func parseArgs(args []string, output io.Writer) (config, error) {
	fs := flag.NewFlagSet("fetch-secrets", flag.ContinueOnError)
	fs.SetOutput(output)
	project := fs.String("project", "", "Google Cloud project that holds the secrets (required)")
	dir := fs.String("dir", "", "directory to write one file per secret into; created 0700 if missing (required)")
	timeout := fs.Duration("timeout", time.Minute, "overall deadline for authenticating and fetching every secret")
	var optional specList
	fs.Var(&optional, "optional", "FILE[=SECRET] of a secret whose absence is tolerated (repeatable)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	cfg := config{project: *project, dir: *dir, timeout: *timeout}
	if cfg.project == "" {
		return config{}, errors.New("-project is required")
	}
	if cfg.dir == "" {
		return config{}, errors.New("-dir is required")
	}
	if cfg.timeout <= 0 {
		return config{}, errors.New("-timeout must be positive")
	}

	seen := map[string]bool{}
	add := func(raw string, isOptional bool) error {
		spec, err := parseSpec(raw, isOptional)
		if err != nil {
			return err
		}
		if seen[spec.file] {
			return fmt.Errorf("file %q is named more than once", spec.file)
		}
		seen[spec.file] = true
		cfg.specs = append(cfg.specs, spec)
		return nil
	}
	for _, raw := range fs.Args() {
		if err := add(raw, false); err != nil {
			return config{}, err
		}
	}
	for _, raw := range optional {
		if err := add(raw, true); err != nil {
			return config{}, err
		}
	}
	if len(cfg.specs) == 0 {
		return config{}, errors.New("no secrets named")
	}
	return cfg, nil
}

// parseSpec parses FILE or FILE=SECRET.
func parseSpec(raw string, optional bool) (secretSpec, error) {
	file, secret, renamed := strings.Cut(raw, "=")
	if !renamed {
		secret = file
	}
	if !fileNamePattern.MatchString(file) {
		return secretSpec{}, fmt.Errorf("%q: file name must be lowercase letters, digits and dashes (it becomes the server flag -<name>-file)", raw)
	}
	if !secretNamePattern.MatchString(secret) {
		return secretSpec{}, fmt.Errorf("%q: %q is not a valid Secret Manager secret name", raw, secret)
	}
	return secretSpec{file: file, secret: secret, optional: optional}, nil
}

// fetcher reads secrets through the Secret Manager REST API.
type fetcher struct {
	client   *http.Client
	endpoint string
	project  string
	log      io.Writer
}

// fetchAll writes every spec's value into dir, failing on the first secret that
// cannot be read. Files are written 0400 into a 0700 directory.
func (f *fetcher) fetchAll(ctx context.Context, dir string, specs []secretSpec) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restrict %s: %w", dir, err)
	}

	for _, spec := range specs {
		value, err := f.access(ctx, spec.secret)
		switch {
		case err == nil:
			value = bytes.TrimRight(value, " \t\r\n")
		case spec.optional && errors.Is(err, errSecretAbsent):
			fmt.Fprintf(f.log, "fetch-secrets: optional secret %q not available (%v); feature disabled\n", spec.secret, err)
			value = nil
		default:
			return fmt.Errorf("secret %q in project %q: %w\n%s", spec.secret, f.project, err, accessHint(spec.secret, f.project))
		}

		if len(value) == 0 && err == nil {
			if !spec.optional {
				return fmt.Errorf("secret %q in project %q is empty (after trimming trailing whitespace)", spec.secret, f.project)
			}
			fmt.Fprintf(f.log, "fetch-secrets: optional secret %q is empty; feature disabled\n", spec.secret)
		}

		if err := writeSecretFile(dir, spec.file, value); err != nil {
			return err
		}
	}
	return nil
}

// access returns the decoded payload of the latest version of a secret.
func (f *fetcher) access(ctx context.Context, secret string) ([]byte, error) {
	u := fmt.Sprintf("%s/v1/projects/%s/secrets/%s/versions/latest:access",
		f.endpoint, url.PathEscape(f.project), url.PathEscape(secret))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		// Best effort: a non-JSON error body still yields the HTTP status below.
		_ = json.Unmarshal(body, &apiErr)
		detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if apiErr.Error.Status != "" {
			detail += " " + apiErr.Error.Status
		}
		if apiErr.Error.Message != "" {
			detail += ": " + apiErr.Error.Message
		}
		if isAbsent(resp.StatusCode, apiErr.Error.Status) {
			return nil, fmt.Errorf("%w: %s", errSecretAbsent, detail)
		}
		return nil, errors.New(detail)
	}

	var payload struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	value, err := base64.StdEncoding.DecodeString(payload.Payload.Data)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return value, nil
}

// isAbsent reports whether an error response means the secret is missing
// rather than unreachable: no such secret or version, no permission to read
// it, or no enabled version (FAILED_PRECONDITION on a disabled or destroyed
// latest version).
func isAbsent(statusCode int, status string) bool {
	switch statusCode {
	case http.StatusNotFound, http.StatusForbidden:
		return true
	case http.StatusBadRequest:
		return status == "FAILED_PRECONDITION"
	}
	return false
}

// accessHint is the operator-facing checklist for a required secret that
// could not be read.
func accessHint(secret, project string) string {
	return fmt.Sprintf(`Check that:
  1. the secret exists:           gcloud secrets describe %[1]s --project=%[2]s
  2. it has an enabled version:   gcloud secrets versions list %[1]s --project=%[2]s
  3. this container's identity holds roles/secretmanager.secretAccessor on it:
                                  gcloud secrets get-iam-policy %[1]s --project=%[2]s`, secret, project)
}

// writeSecretFile writes value to dir/name at mode 0400. It writes a hidden
// temporary file and renames it into place, so a reader globbing dir/* never
// sees a partial value, and an existing 0400 file from an earlier run is
// replaced rather than refused.
func writeSecretFile(dir, name string, value []byte) error {
	tmp, err := os.CreateTemp(dir, "."+name+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	// Cleans up after any failure below. After a successful rename the path no
	// longer exists, so the error is expected and ignored.
	defer os.Remove(tmp.Name())

	// On a failed write or chmod the close error is ignored: the earlier error
	// is the one worth reporting, and the deferred remove discards the file.
	if _, err := tmp.Write(value); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Chmod(0o400); err != nil {
		tmp.Close()
		return fmt.Errorf("restrict %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	return nil
}
