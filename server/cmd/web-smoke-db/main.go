// web-smoke-db boots a Postgres container for the Web Smoke CI workflow
// via testcontainers-go and prints its connection string to stdout.
//
// The self-hosted GitHub Actions runner is itself a Docker container that
// mounts /var/run/docker.sock to launch sibling containers. A
// docker-compose Postgres is reachable on the *host's* loopback, not on
// the runner container's — which is why `--db=postgres://...@localhost:5432`
// returned `connection refused` even after Postgres was Healthy. The Go
// test suite (server/storage/testing.go) sidesteps this by going through
// testcontainers-go, which asks the Docker daemon for the container's
// reachable host/port from the calling process's perspective. This helper
// reuses that same path.
//
// The container is named via SMOKE_DB_NAME (or a GITHUB_RUN_ID-derived
// default) so the workflow's cleanup step can remove exactly this
// container. Ryuk is disabled so the container outlives this short-lived
// process — the workflow's `if: always()` cleanup is the only reaper.
//
// Usage:
//
//	go run ./server/cmd/web-smoke-db > /tmp/conn.txt
//	# … run server against $(cat /tmp/conn.txt) …
//	docker rm -fv "$SMOKE_DB_NAME"
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func main() {
	if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
		log.Fatalf("disable ryuk: %v", err)
	}

	name := containerName()

	ctx := context.Background()
	container, err := postgres.Run(ctx,
		"pgvector/pgvector:pg16",
		postgres.WithDatabase("ripls"),
		postgres.WithUsername("ripls"),
		postgres.WithPassword("ripls_dev"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(name),
	)
	if err != nil {
		log.Fatalf("start postgres container %q: %v", name, err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	// Diagnostics go to stderr; only the connection string lands on stdout
	// so callers can do `URL=$(go run ...)`.
	fmt.Fprintf(os.Stderr, "web-smoke-db: container %q ready\n", name)
	fmt.Println(connStr)
}

// containerName returns the Postgres testcontainer name for this run.
//
// Mirrors server/storage/testing.go's testContainerName() pattern: in CI
// the name is derived from GITHUB_RUN_ID / GITHUB_RUN_ATTEMPT / GITHUB_JOB
// so the workflow's cleanup step can target exactly this container. An
// explicit SMOKE_DB_NAME overrides both.
func containerName() string {
	if v := os.Getenv("SMOKE_DB_NAME"); v != "" {
		return v
	}
	runID := os.Getenv("GITHUB_RUN_ID")
	if runID == "" {
		runID = "local"
	}
	attempt := os.Getenv("GITHUB_RUN_ATTEMPT")
	if attempt == "" {
		attempt = "1"
	}
	job := os.Getenv("GITHUB_JOB")
	if job == "" {
		job = "web-smoke"
	}
	return fmt.Sprintf("ripls-web-smoke-pg-%s-%s-%s", runID, attempt, job)
}
