package storage

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

// TestParseContainerPPID verifies the name → PPID parser handles the shapes
// we care about and rejects malformed input rather than killing the wrong
// containers.
func TestParseContainerPPID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantPID int
		wantOK  bool
	}{
		{"standard name", "/ripls-test-postgres-12345", 12345, true},
		{"missing slash", "ripls-test-postgres-12345", 0, false},
		{"wrong prefix", "/other-postgres-12345", 0, false},
		{"non-numeric suffix", "/ripls-test-postgres-abc", 0, false},
		{"zero pid", "/ripls-test-postgres-0", 0, false},
		{"negative pid", "/ripls-test-postgres--1", 0, false},
		{"empty suffix", "/ripls-test-postgres-", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, ok := parseContainerPPID(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v want %v", ok, tt.wantOK)
			}
			if pid != tt.wantPID {
				t.Errorf("pid: got %v want %v", pid, tt.wantPID)
			}
		})
	}
}

// TestProcessExists sanity-checks the liveness probe against our own PID
// (must be alive) and a PID we're confident is dead.
func TestProcessExists(t *testing.T) {
	if !processExists(os.Getpid()) {
		t.Errorf("processExists(own pid) = false, want true")
	}
	// PID 1 always exists on Unix and we don't own it; processExists should
	// still return true because the probe should not confuse permission
	// errors with nonexistence.
	if !processExists(1) {
		t.Errorf("processExists(1) = false, want true (should treat EPERM as alive)")
	}
	// Use a huge PID that shouldn't exist on any reasonable system.
	if processExists(99999999) {
		t.Errorf("processExists(99999999) = true, want false")
	}
}

// TestLogContainerDiagnostics_NoopWhenUninitialized verifies the diagnostic
// helper is safe to call before the shared container has been set up. The
// sync.Once guard should also keep multiple calls harmless.
func TestLogContainerDiagnostics_NoopWhenUninitialized(t *testing.T) {
	sharedContainerName = ""
	containerDiagnosticOnce = onceReset()

	logContainerDiagnostics(t)
	logContainerDiagnostics(t)
}

// TestDumpDockerEvents_NoRunStart verifies dumpDockerEvents returns cleanly
// when runStart is zero (the shared container never came up).
func TestDumpDockerEvents_NoRunStart(t *testing.T) {
	dockerClient, err := newDockerAPIClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer func() { _ = dockerClient.Close() }()

	dumpDockerEvents(t, dockerClient, "ripls-test-nonexistent", time.Time{})
}

// TestDumpContainerLogs_MissingContainer verifies dumpContainerLogs logs a
// diagnostic and returns without panicking when the container ID doesn't
// exist.
func TestDumpContainerLogs_MissingContainer(t *testing.T) {
	dockerClient, err := newDockerAPIClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer func() { _ = dockerClient.Close() }()

	dumpContainerLogs(t, dockerClient, "0000000000000000000000000000000000000000000000000000000000000000")
}

// onceReset returns a fresh sync.Once. Used in tests to exercise the
// sync.Once-guarded diagnostic function multiple times per test run.
func onceReset() sync.Once { return sync.Once{} }

// TestCleanupOldTestContainers_RemovesOrphanWithVolume verifies the local
// orphan sweep reaps a container whose owning PPID is dead AND removes the
// anonymous volume Docker created for it. This is the end-to-end shape of the
// pgvector test-container leak.
func TestCleanupOldTestContainers_RemovesOrphanWithVolume(t *testing.T) {
	dockerClient, err := newDockerAPIClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer func() { _ = dockerClient.Close() }()
	ctx := context.Background()

	// Use a guaranteed-dead PPID so processExists() returns false and the
	// orphan reaper acts on this container.
	const deadPPID = 99999999
	containerName := "ripls-test-postgres-" + strconv.Itoa(deadPPID)

	// Best-effort cleanup of any prior leftover from a failed run.
	_ = dockerClient.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true, RemoveVolumes: true})

	// Use alpine: tiny, no entrypoint that lingers, and accepts a VOLUME mount
	// at any path. The `Volumes` map (no Source) tells Docker to create an
	// anonymous volume — exactly how pgvector inherits its data volume.
	created, err := dockerClient.ContainerCreate(ctx,
		&container.Config{
			Image:   "alpine:3",
			Cmd:     []string{"sh", "-c", "sleep 3600"},
			Volumes: map[string]struct{}{"/data": {}},
		},
		nil, nil, nil, containerName)
	if err != nil {
		t.Skipf("could not create test container (image pull may have failed): %v", err)
	}

	// Capture the anonymous volume name so we can assert it's gone after.
	inspect, err := dockerClient.ContainerInspect(ctx, created.ID)
	if err != nil {
		_ = dockerClient.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		t.Fatalf("inspect: %v", err)
	}
	var anonVolName string
	for _, m := range inspect.Mounts {
		if m.Type == "volume" {
			anonVolName = m.Name
			break
		}
	}
	if anonVolName == "" {
		_ = dockerClient.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		t.Fatal("expected anonymous volume mount on container, got none")
	}
	t.Cleanup(func() {
		_ = dockerClient.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		_ = dockerClient.VolumeRemove(ctx, anonVolName, true)
	})

	// Reset the once guard so cleanupOldTestContainers actually runs.
	cleanupOnce = sync.Once{}
	cleanupOldTestContainers()

	// Container must be gone.
	if _, err := dockerClient.ContainerInspect(ctx, created.ID); err == nil {
		t.Errorf("orphan container %s was not removed", containerName)
	}

	// Anonymous volume must be gone (either removed with the container via
	// RemoveVolumes:true, or by the dangling-volume sweep).
	listed, err := dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", anonVolName)),
	})
	if err != nil {
		t.Fatalf("volume list: %v", err)
	}
	for _, v := range listed.Volumes {
		if v != nil && v.Name == anonVolName {
			t.Errorf("anonymous volume %s for orphan container was not removed", anonVolName)
		}
	}
}

// TestPruneDanglingAnonymousVolumes verifies the local-cleanup volume sweep
// removes orphaned anonymous volumes (the kind pgvector test containers leave
// behind when removed without `-v`) while leaving non-anonymous and
// non-dangling volumes alone.
func TestPruneDanglingAnonymousVolumes(t *testing.T) {
	dockerClient, err := newDockerAPIClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer func() { _ = dockerClient.Close() }()
	ctx := context.Background()

	// Volume A: dangling + anonymous → must be removed.
	anonName := "ripls-test-prune-anon-" + strconv.Itoa(os.Getpid())
	anonVol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{
		Name:   anonName,
		Labels: map[string]string{"com.docker.volume.anonymous": ""},
	})
	if err != nil {
		t.Fatalf("create anon volume: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.VolumeRemove(ctx, anonVol.Name, true) })

	// Volume B: dangling but NOT anonymous (named user volume) → must survive.
	namedName := "ripls-test-prune-named-" + strconv.Itoa(os.Getpid())
	namedVol, err := dockerClient.VolumeCreate(ctx, volume.CreateOptions{
		Name: namedName,
	})
	if err != nil {
		t.Fatalf("create named volume: %v", err)
	}
	t.Cleanup(func() { _ = dockerClient.VolumeRemove(ctx, namedVol.Name, true) })

	pruneDanglingAnonymousVolumes(ctx, dockerClient)

	// The anonymous one must be gone.
	listed, err := dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", anonName)),
	})
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	for _, v := range listed.Volumes {
		if v != nil && v.Name == anonName {
			t.Errorf("anonymous dangling volume %q was not removed", anonName)
		}
	}

	// The named one must still be present.
	listed, err = dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", namedName)),
	})
	if err != nil {
		t.Fatalf("list named after prune: %v", err)
	}
	found := false
	for _, v := range listed.Volumes {
		if v != nil && v.Name == namedName {
			found = true
		}
	}
	if !found {
		t.Errorf("named volume %q was incorrectly removed", namedName)
	}
}

// TestTestContainerName verifies the container name is stable per CI job
// (so the workflow post-step can clean up exactly one container) and falls
// back to PPID in local dev (so concurrent dev runs don't collide).
func TestTestContainerName(t *testing.T) {
	saved := map[string]string{}
	for _, k := range []string{"GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_JOB"} {
		saved[k] = os.Getenv(k)
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	})

	t.Run("local dev uses PPID", func(t *testing.T) {
		got := testContainerName()
		want := "ripls-test-postgres-" + strconv.Itoa(os.Getppid())
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("CI uses run/job identifiers", func(t *testing.T) {
		_ = os.Setenv("GITHUB_RUN_ID", "12345")
		_ = os.Setenv("GITHUB_RUN_ATTEMPT", "2")
		_ = os.Setenv("GITHUB_JOB", "test")
		got := testContainerName()
		want := "ripls-test-postgres-ci-12345-2-test"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("CI defaults missing attempt and job", func(t *testing.T) {
		_ = os.Setenv("GITHUB_RUN_ID", "999")
		_ = os.Unsetenv("GITHUB_RUN_ATTEMPT")
		_ = os.Unsetenv("GITHUB_JOB")
		got := testContainerName()
		want := "ripls-test-postgres-ci-999-1-job"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
