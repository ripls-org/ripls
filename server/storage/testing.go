package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"go.ripls.org/ripls/server/ai/embedding"
)

// Shared PostgreSQL testcontainer for all tests.
var (
	sharedPostgresOnce      sync.Once
	sharedBaseConnString    string // Connection string to postgres database (for creating new DBs)
	sharedContainerName     string // Container name for diagnostics
	sharedContainerStart    time.Time
	errSetup                error
	setupFailureDiagnostics string // Captured at setup failure, replayed on each test
	cleanupOnce             sync.Once
	containerDiagnosticOnce sync.Once
	setupDiagnosticOnce     sync.Once
)

// isCI reports whether tests are running in a CI environment. Checks
// multiple env vars because self-hosted GitHub Actions runners don't always
// set CI=true (depends on runner configuration), but GITHUB_ACTIONS=true is
// a documented guarantee when the workflow runs.
func isCI() bool {
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "JENKINS_URL"} {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}

// testContainerName returns the postgres testcontainer name for this run.
//
// In CI, the name is derived from per-job identifiers (GITHUB_RUN_ID +
// GITHUB_RUN_ATTEMPT + GITHUB_JOB) so the workflow's post-step can remove
// exactly this container — and only this container — without affecting
// concurrent jobs that share the host Docker daemon. PPID is unsuitable in
// CI because (a) self-hosted runners share a Docker daemon across
// concurrent job containers each in its own PID namespace, and (b) ryuk
// is disabled in CI to avoid create-races between parallel test packages,
// so nothing reaps unnamed sessions automatically.
//
// In local dev, falls back to PPID so that all packages spawned by one
// `go test ./...` share a container, and `cleanupOldTestContainers` can
// reap orphans whose `go test` parent has exited.
func testContainerName() string {
	if runID := os.Getenv("GITHUB_RUN_ID"); runID != "" {
		attempt := os.Getenv("GITHUB_RUN_ATTEMPT")
		if attempt == "" {
			attempt = "1"
		}
		job := os.Getenv("GITHUB_JOB")
		if job == "" {
			job = "job"
		}
		return fmt.Sprintf("ripls-test-postgres-ci-%s-%s-%s", runID, attempt, job)
	}
	return fmt.Sprintf("ripls-test-postgres-%d", os.Getppid())
}

// cleanupOldTestContainers removes genuinely orphaned test containers — those
// whose owning `go test` process (identified by PPID encoded in the container
// name) is no longer alive. This is called once at the start of a test run.
//
// The previous implementation removed every `ripls-test-postgres-*` container
// whose name didn't match the current PPID, which forcibly SIGKILLed containers
// belonging to *concurrently running* test processes on shared runners. That
// caused rapid create/start/kill cycles that never let Postgres finish initdb
// and produced the "matched 0 times, expected 2" readiness-wait failures the
// team was seeing on the self-hosted runner.
//
// Also disables the ryuk reaper so multiple parallel test packages don't race
// to create it.
//
// This runs only in local-dev (see setupSharedPostgreSQL). In CI, concurrent
// runs live in separate PID namespaces so the liveness check can't tell
// "dead" from "visible in my namespace" — the safe default there is to skip
// cleanup entirely and rely on the runner to isolate job state.
func cleanupOldTestContainers() {
	cleanupOnce.Do(func() {
		// Recover from panics - cleanup is best-effort and should never fail tests
		defer func() {
			_ = recover()
		}()

		// Disable ryuk reaper to prevent race conditions when multiple test packages
		// try to create it simultaneously. Cleanup is handled by this function instead.
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		ctx := context.Background()
		currentContainerName := testContainerName()

		// Use Docker client to find and remove old test containers
		dockerClient, err := newDockerAPIClient()
		if err != nil {
			return // Silently ignore - cleanup is best-effort
		}
		defer func() { _ = dockerClient.Close() }()

		// List all containers matching our naming pattern
		containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("name", "ripls-test-postgres-")),
		})
		if err != nil {
			return
		}

		// Remove containers whose owning `go test` PPID is no longer alive.
		// Skip the current run's container and any concurrent run whose
		// process is still going.
		for _, c := range containers {
			for _, name := range c.Names {
				// Container names have a leading slash
				if name == "/"+currentContainerName {
					continue
				}
				ppid, ok := parseContainerPPID(name)
				if !ok {
					// Name didn't parse — leave it alone rather than risk
					// killing something we don't understand.
					continue
				}
				if processExists(ppid) {
					// A live `go test` process owns this container; it's in
					// active use by a concurrent run. Must not touch it.
					continue
				}
				_ = dockerClient.ContainerRemove(ctx, c.ID, container.RemoveOptions{
					Force:         true,
					RemoveVolumes: true,
				})
			}
		}

		// Sweep dangling anonymous volumes left behind by prior runs that were
		// removed without `-v` (e.g., old CI behavior, manual `docker rm`,
		// crashes between create and register). pgvector/pgvector:pg16
		// inherits VOLUME /var/lib/postgresql/data, so every container creates
		// an anonymous volume that persists if not explicitly removed.
		// Restricting to dangling+anonymous keeps named user volumes safe.
		pruneDanglingAnonymousVolumes(ctx, dockerClient)
	})
}

// newDockerAPIClient builds a github.com/docker/docker API client using the
// standard DOCKER_HOST / socket resolution with API-version negotiation.
//
// testcontainers v0.42 moved its provider client to the redesigned, pre-1.0
// github.com/moby/moby/client. The cleanup and diagnostic helpers here stay on
// the stable github.com/docker/docker API and construct their own client
// rather than borrowing the provider's, so a future testcontainers SDK change
// can't break them. These helpers are all best-effort, so if the daemon is
// unreachable the caller simply skips its work.
func newDockerAPIClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// pruneDanglingAnonymousVolumes removes Docker volumes that are both
// dangling (no container attached) and anonymous (created by an image's
// VOLUME directive rather than `docker volume create`). Best-effort.
func pruneDanglingAnonymousVolumes(ctx context.Context, dockerClient client.APIClient) {
	defer func() { _ = recover() }()

	vols, err := dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("dangling", "true"),
			filters.Arg("label", "com.docker.volume.anonymous"),
		),
	})
	if err != nil {
		return
	}
	for _, v := range vols.Volumes {
		if v == nil {
			continue
		}
		_ = dockerClient.VolumeRemove(ctx, v.Name, false)
	}
}

// parseContainerPPID extracts the PPID suffix from a container name of the
// form "/ripls-test-postgres-<pid>". Returns (0, false) on any parse failure.
func parseContainerPPID(name string) (int, bool) {
	const prefix = "/ripls-test-postgres-"
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	rest := name[len(prefix):]
	pid, err := strconv.Atoi(rest)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// processExists reports whether a process with the given PID is currently
// alive. Uses signal 0 on POSIX (no-op signal that only performs existence/
// permission checks). Returns true on permission-denied since that still
// proves the process exists.
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// ESRCH / os.ErrProcessDone both mean "no such process"; any other
	// error (e.g. EPERM) means the process exists but we don't own it.
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
		return false
	}
	return true
}

// setupSharedPostgreSQL initializes a shared PostgreSQL testcontainer for all tests.
// Uses testcontainers with pgvector/pgvector:pg16 image for fast startup and pgvector support.
// The container is reused across all test packages using WithReuseByName, which prevents
// Docker from being overwhelmed when running tests in parallel.
func setupSharedPostgreSQL() error {
	// Clean up orphaned containers from previous test runs — but ONLY in
	// local-dev. In CI (self-hosted runners sharing a Docker daemon across
	// concurrent jobs), the PPID-based liveness check can't see processes
	// in other jobs' PID namespaces, so cleanup incorrectly SIGKILLs pg
	// containers belonging to concurrent runs (see #1238). CI runners are
	// expected to isolate job state themselves, so skipping cleanup there is safe.
	if isCI() {
		// Still disable ryuk — multiple parallel test packages within the
		// same CI job otherwise race to create it.
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	} else {
		cleanupOldTestContainers()
	}

	sharedPostgresOnce.Do(func() {
		ctx := context.Background()

		// All packages spawned by the same test invocation share one container;
		// different invocations get distinct containers. See testContainerName
		// for how the name is derived in CI vs local dev.
		containerName := testContainerName()

		// Record setup-attempt start for event queries and diagnostic output.
		setupStart := time.Now()
		sharedContainerName = containerName
		sharedContainerStart = setupStart

		container, err := postgres.Run(ctx,
			"pgvector/pgvector:pg16",
			postgres.WithDatabase("postgres"), // Use default postgres database
			postgres.WithUsername("testuser"),
			postgres.WithPassword("testpass"),
			postgres.BasicWaitStrategies(),
			testcontainers.WithReuseByName(containerName),
		)
		if err != nil {
			errSetup = fmt.Errorf("failed to start PostgreSQL container: %w", err)
			// Capture whatever we can about the failed container — Postgres
			// stderr is the authoritative source for "why didn't initdb
			// finish". Even if testcontainers tore the container down, logs
			// and events for the remnant may still be retrievable.
			setupFailureDiagnostics = captureSetupFailureDiagnostics(containerName, setupStart)
			return
		}

		connStr, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			errSetup = fmt.Errorf("failed to get connection string: %w", err)
			return
		}

		sharedBaseConnString = connStr
		sharedContainerName = containerName
		sharedContainerStart = time.Now()
	})
	return errSetup
}

// logContainerDiagnostics inspects the Docker container state when a connection
// failure occurs. Runs at most once per test invocation so the diagnostic
// output isn't repeated for every subsequent failing test. Captures:
//   - Inspect result (or remnant state from ContainerList when inspect fails)
//   - Container logs (Postgres stderr often contains FATAL / OOM messages)
//   - Docker events since run start (kill/die/destroy events with exit codes)
func logContainerDiagnostics(t *testing.T) {
	containerDiagnosticOnce.Do(func() {
		if sharedContainerName == "" {
			return
		}

		ctx := context.Background()
		dockerClient, err := newDockerAPIClient()
		if err != nil {
			t.Logf("CONTAINER DIAGNOSTIC: could not create Docker client: %v", err)
			return
		}
		defer func() { _ = dockerClient.Close() }()

		var containerID string

		inspect, err := dockerClient.ContainerInspect(ctx, sharedContainerName)
		if err != nil {
			t.Logf("CONTAINER DIAGNOSTIC: inspect failed (container may be gone): %v", err)
			// Container may have been removed. Try to find any remnant by
			// listing all containers (including stopped/dead) matching the
			// name prefix — this sometimes surfaces an Exited container we
			// can still log from.
			remnants, listErr := dockerClient.ContainerList(ctx, container.ListOptions{
				All:     true,
				Filters: filters.NewArgs(filters.Arg("name", sharedContainerName)),
			})
			if listErr != nil {
				t.Logf("CONTAINER DIAGNOSTIC: container list also failed: %v", listErr)
			} else if len(remnants) == 0 {
				t.Logf("CONTAINER DIAGNOSTIC: no container remnants found for name=%s", sharedContainerName)
			} else {
				for _, c := range remnants {
					t.Logf("CONTAINER DIAGNOSTIC: remnant id=%s state=%s status=%s image=%s",
						c.ID[:min(12, len(c.ID))], c.State, c.Status, c.Image)
					containerID = c.ID
				}
			}
		} else {
			containerID = inspect.ID
			t.Logf("CONTAINER DIAGNOSTIC: name=%s status=%s running=%v",
				sharedContainerName, inspect.State.Status, inspect.State.Running)
			if inspect.State.OOMKilled {
				t.Logf("CONTAINER DIAGNOSTIC: *** OOM KILLED ***")
			}
			if inspect.State.ExitCode != 0 {
				t.Logf("CONTAINER DIAGNOSTIC: exit_code=%d error=%s finished_at=%s",
					inspect.State.ExitCode, inspect.State.Error, inspect.State.FinishedAt)
			}
			if inspect.HostConfig != nil {
				t.Logf("CONTAINER DIAGNOSTIC: shm_size=%dMB memory_limit=%dMB",
					inspect.HostConfig.ShmSize/(1024*1024),
					inspect.HostConfig.Memory/(1024*1024))
			}
		}

		// Dump container logs (best-effort). Postgres FATAL messages, crash
		// output, and signal-death notices go to stderr. Works on both
		// running and exited containers as long as the container record
		// still exists.
		if containerID != "" {
			dumpContainerLogs(t, dockerClient, containerID)
		}

		// Dump Docker events since run start (best-effort). Captures
		// kill/die/destroy events with exit codes and signals — the
		// authoritative source for "who killed this container".
		dumpDockerEvents(t, dockerClient, sharedContainerName, sharedContainerStart)
	})
}

// captureSetupFailureDiagnostics runs when postgres.Run() fails. It queries
// Docker directly (bypassing testcontainers) to find any remnants of the
// container we tried to create, pulls their logs and records, and returns
// a single formatted string that subsequent tests can surface via t.Log.
// Best-effort: any step may fail silently. Never panics.
func captureSetupFailureDiagnostics(containerName string, setupStart time.Time) string {
	defer func() { _ = recover() }()

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: postgres.Run failed for container=%s (started at %s)\n",
		containerName, setupStart.Format(time.RFC3339))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dockerClient, err := newDockerAPIClient()
	if err != nil {
		fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: could not create docker client: %v\n", err)
		return buf.String()
	}
	defer func() { _ = dockerClient.Close() }()

	// List any containers matching the failed name (all states).
	list, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", containerName)),
	})
	if err != nil {
		fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: container list failed: %v\n", err)
	} else if len(list) == 0 {
		fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: no container found with name=%s (testcontainers already removed it)\n", containerName)
	} else {
		for _, c := range list {
			shortID := c.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: remnant id=%s state=%s status=%s image=%s\n",
				shortID, c.State, c.Status, c.Image)

			// Pull logs for the remnant.
			if logs := fetchContainerLogsString(dockerClient, c.ID); logs != "" {
				fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: --- logs for %s ---\n%s", shortID, logs)
			}
		}
	}

	// Events since setup start — captures pulls, creates, starts, dies, oom.
	if ev := fetchDockerEventsString(dockerClient, containerName, setupStart); ev != "" {
		fmt.Fprintf(&buf, "SETUP DIAGNOSTIC: --- events since %s ---\n%s",
			setupStart.Format(time.RFC3339), ev)
	}

	return buf.String()
}

// fetchContainerLogsString returns the tail of a container's stdout+stderr
// as a single string, or empty on any error.
func fetchContainerLogsString(dockerClient client.APIClient, containerID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reader, err := dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "200",
		Timestamps: true,
	})
	if err != nil {
		return fmt.Sprintf("(log fetch error: %v)\n", err)
	}
	defer reader.Close()

	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, reader)

	var out bytes.Buffer
	if stdout.Len() > 0 {
		fmt.Fprintf(&out, "  [stdout]\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		fmt.Fprintf(&out, "  [stderr]\n%s", stderr.String())
	}
	if out.Len() == 0 {
		return "(logs empty)\n"
	}
	return out.String()
}

// fetchDockerEventsString returns a formatted list of docker events for the
// container since setupStart, or empty on any error / no events.
func fetchDockerEventsString(dockerClient client.APIClient, containerName string, setupStart time.Time) string {
	if setupStart.IsZero() {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evFilter := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("container", containerName),
	)
	evCh, errCh := dockerClient.Events(ctx, events.ListOptions{
		Since:   fmt.Sprintf("%d", setupStart.Unix()),
		Until:   fmt.Sprintf("%d", time.Now().Unix()),
		Filters: evFilter,
	})

	var lines []string
	for {
		select {
		case <-ctx.Done():
			goto done
		case err := <-errCh:
			if err != nil && !strings.Contains(err.Error(), "EOF") {
				lines = append(lines, fmt.Sprintf("  (event stream error: %v)", err))
			}
			goto done
		case ev, ok := <-evCh:
			if !ok {
				goto done
			}
			ts := time.Unix(0, ev.TimeNano).UTC().Format(time.RFC3339Nano)
			extra := ""
			if exit, present := ev.Actor.Attributes["exitCode"]; present {
				extra += " exit_code=" + exit
			}
			if sig, present := ev.Actor.Attributes["signal"]; present {
				extra += " signal=" + sig
			}
			lines = append(lines, fmt.Sprintf("  %s action=%s%s", ts, ev.Action, extra))
		}
	}
done:
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// dumpContainerLogs pulls the tail of a container's stdout/stderr via the
// Docker API and demultiplexes the stdcopy frames. Best-effort: logs a note
// and returns on any error instead of failing the test.
func dumpContainerLogs(t *testing.T, dockerClient client.APIClient, containerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reader, err := dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "200",
		Timestamps: true,
	})
	if err != nil {
		t.Logf("CONTAINER DIAGNOSTIC: could not read container logs: %v", err)
		return
	}
	defer reader.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, reader); err != nil {
		// Non-fatal: partial output is still useful.
		t.Logf("CONTAINER DIAGNOSTIC: log stream ended with error: %v", err)
	}

	if stdout.Len() > 0 {
		t.Logf("CONTAINER DIAGNOSTIC: --- container stdout (tail 200) ---\n%s", stdout.String())
	}
	if stderr.Len() > 0 {
		t.Logf("CONTAINER DIAGNOSTIC: --- container stderr (tail 200) ---\n%s", stderr.String())
	}
	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Logf("CONTAINER DIAGNOSTIC: container logs were empty")
	}
}

// dumpDockerEvents pulls Docker daemon events for the named container since
// runStart. Captures kill/die/destroy/oom events — the authoritative source
// for "who killed this container and why". Best-effort.
func dumpDockerEvents(t *testing.T, dockerClient client.APIClient, containerName string, runStart time.Time) {
	if runStart.IsZero() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evFilter := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("container", containerName),
	)

	evCh, errCh := dockerClient.Events(ctx, events.ListOptions{
		Since:   fmt.Sprintf("%d", runStart.Unix()),
		Until:   fmt.Sprintf("%d", time.Now().Unix()),
		Filters: evFilter,
	})

	var collected []string
	for {
		select {
		case <-ctx.Done():
			goto done
		case err := <-errCh:
			if err != nil && !strings.Contains(err.Error(), "EOF") {
				t.Logf("CONTAINER DIAGNOSTIC: event stream error: %v", err)
			}
			goto done
		case ev, ok := <-evCh:
			if !ok {
				goto done
			}
			ts := time.Unix(0, ev.TimeNano).UTC().Format(time.RFC3339Nano)
			extra := ""
			if exit, present := ev.Actor.Attributes["exitCode"]; present {
				extra += " exit_code=" + exit
			}
			if sig, present := ev.Actor.Attributes["signal"]; present {
				extra += " signal=" + sig
			}
			collected = append(collected, fmt.Sprintf("  %s action=%s%s", ts, ev.Action, extra))
		}
	}
done:
	if len(collected) == 0 {
		t.Logf("CONTAINER DIAGNOSTIC: no docker events recorded for container=%s since %s",
			containerName, runStart.Format(time.RFC3339))
		return
	}
	t.Logf("CONTAINER DIAGNOSTIC: --- docker events for %s since %s ---\n%s",
		containerName, runStart.Format(time.RFC3339), strings.Join(collected, "\n"))
}

// CleanupSharedPostgreSQL is a no-op when using container reuse.
// The container is shared across test packages via WithReuseByName, so we
// can't terminate it from any single test process — other packages may
// still be using it. Lifecycle:
//
//   - Local dev: cleaned up on the next `go test ./...` invocation by
//     cleanupOldTestContainers, which reaps containers whose owning PPID
//     is no longer alive. ryuk also runs as a backstop.
//   - CI: container name is stable per GitHub Actions job (see
//     testContainerName), and the workflow's post-step removes it
//     unconditionally with `docker rm -f`. ryuk is disabled in CI to
//     avoid races when parallel test packages create it concurrently.
func CleanupSharedPostgreSQL() {
	// No-op: see comment above.
}

// createTestDatabase creates a new unique database for this test.
func createTestDatabase(t *testing.T) (string, func()) {
	// Generate unique database name using UUID (replace hyphens with underscores for valid SQL identifier)
	dbName := "test_" + strings.ReplaceAll(uuid.New().String(), "-", "_")

	// Connect to the postgres database to create a new database
	db, err := sql.Open("postgres", sharedBaseConnString)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			logContainerDiagnostics(t)
		}
		t.Fatalf("Failed to connect to postgres: %v", err)
	}

	// Create the new database. dbName is a generated UUID, but it still goes
	// through quoteIdent: this file is not a _test.go file, so it is linted and
	// read as production code, and it used to be the one place in the tree that
	// spliced an unvalidated name into DDL.
	quotedDB, err := quoteIdent(dbName)
	if err != nil {
		db.Close()
		t.Fatalf("Generated test database name is not a valid SQL identifier: %v", err)
	}
	_, err = db.Exec(fmt.Sprintf(`CREATE DATABASE %s`, quotedDB))
	if err != nil {
		db.Close()
		if strings.Contains(err.Error(), "connection refused") {
			logContainerDiagnostics(t)
		}
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.Close()

	// Build connection string for the new database
	// Replace "/postgres?" with "/dbName?" in the connection string
	connStr := strings.Replace(sharedBaseConnString, "/postgres?", "/"+dbName+"?", 1)

	cleanup := func() {
		// Connect to postgres database to drop the test database
		db, err := sql.Open("postgres", sharedBaseConnString)
		if err != nil {
			t.Logf("Warning: failed to connect for cleanup: %v", err)
			return
		}
		defer db.Close()

		// Terminate any connections to the database before dropping. datname is
		// a value here, not an identifier, so it binds rather than interpolates.
		_, _ = db.Exec(`
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()
		`, dbName)

		_, err = db.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, quotedDB))
		if err != nil {
			t.Logf("Warning: failed to drop test database: %v", err)
		}
	}

	return connStr, cleanup
}

// SetupTestDatabase creates a unique PostgreSQL database for testing.
// Returns the connection string and a cleanup function.
// Use this for integration tests that need a database URL but manage their own connections.
func SetupTestDatabase(t *testing.T) (string, func()) {
	// Initialize the shared PostgreSQL container if not already started
	if err := setupSharedPostgreSQL(); err != nil {
		logSetupFailureDiagnosticsOnce(t)
		t.Fatalf("Failed to setup shared PostgreSQL: %v", err)
	}

	// Create a unique database for this test
	return createTestDatabase(t)
}

// logSetupFailureDiagnosticsOnce emits the forensic output captured at
// setup time. Guarded so the dump appears once per test process even when
// dozens of downstream tests all hit the same errSetup.
func logSetupFailureDiagnosticsOnce(t *testing.T) {
	setupDiagnosticOnce.Do(func() {
		if setupFailureDiagnostics != "" {
			t.Log(setupFailureDiagnostics)
		}
	})
}

// SetupTestStorage creates a PostgreSQL storage instance for testing.
// Each test gets its own unique database for complete isolation.
func SetupTestStorage(t *testing.T) (*ProtoSQLStorage, func()) {
	// Create a unique database for this test
	connStr, dbCleanup := SetupTestDatabase(t)

	// Create a new storage instance connected to the test database
	sqlStorage, err := InitializePostgreSQLDatabase(t.Context(), connStr, DefaultStorageTypes())
	if err != nil {
		dbCleanup()
		t.Fatalf("Failed to initialize PostgreSQL test database: %v", err)
	}

	cleanup := func() {
		sqlStorage.Close()
		dbCleanup()
	}

	return sqlStorage, cleanup
}

// SetupTestEmbedder creates a real embedder for testing.
// Returns nil if the model files are not available (skips embedding tests).
// The model files are expected to be in the model_tuning directory relative to the project root.
func SetupTestEmbedder(t *testing.T) *embedding.Embedder {
	// Find project root by looking for go.mod from the server directory
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("Skipping embedding test: could not determine project root")
		return nil
	}

	// Go up from server/storage/testing.go to project root
	projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	modelPath := filepath.Join(projectRoot, "model_tuning", "ripls_embedding.onnx")
	vocabPath := filepath.Join(projectRoot, "model_tuning", "ripls_embedding_tokenizer", "vocab.txt")

	// Check if model files exist
	if _, err := os.Stat(modelPath); err != nil {
		t.Skipf("Skipping embedding test: model file not found at %s", modelPath)
		return nil
	}
	if _, err := os.Stat(vocabPath); err != nil {
		t.Skipf("Skipping embedding test: vocab file not found at %s", vocabPath)
		return nil
	}

	embedder, err := embedding.New(modelPath, vocabPath)
	if err != nil {
		t.Skipf("Skipping embedding test: could not initialize embedder: %v", err)
		return nil
	}

	t.Cleanup(func() {
		embedder.Close()
	})

	return embedder
}
