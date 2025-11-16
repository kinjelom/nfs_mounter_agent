package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// resetPrometheusRegistry ensures each test has a fresh registry so that
// promauto.* metric registration does not panic with duplicate names.
func resetPrometheusRegistry(t *testing.T) {
	t.Helper()
	r := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = r
	prometheus.DefaultGatherer = r
}

func TestNewWatchdogInitialState(t *testing.T) {
	resetPrometheusRegistry(t)

	points := []string{"/mnt/a", "/mnt/b"}
	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, time.Second, false)

	// lastHealthy should have an entry for each mount point, default false
	if len(w.lastHealthy) != len(points) {
		t.Fatalf("expected lastHealthy length %d, got %d", len(points), len(w.lastHealthy))
	}

	for _, mp := range points {
		healthy, ok := w.IsMountHealthy(mp)
		if !ok {
			t.Errorf("expected IsMountHealthy to know about mount point %q", mp)
		}
		if healthy {
			t.Errorf("expected mount point %q to be unhealthy by default", mp)
		}
	}

	// With two mountpoints defaulting to false, IsHealthy should be false.
	if w.IsHealthy() {
		t.Errorf("expected IsHealthy() to be false with all mount points unhealthy")
	}

	// When enableWriteTest is false, write-test metric should not be created.
	if w.nfsWriteTestDuration != nil {
		t.Errorf("expected nfsWriteTestDuration to be nil when enableWriteTest=false")
	}
}

func TestNewWatchdogWithWriteTestMetric(t *testing.T) {
	resetPrometheusRegistry(t)

	points := []string{"/mnt/a"}
	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, time.Second, true)

	if w.nfsWriteTestDuration == nil {
		t.Fatalf("expected nfsWriteTestDuration to be non-nil when enableWriteTest=true")
	}
}

func TestSetHealthyAndIsHealthy(t *testing.T) {
	resetPrometheusRegistry(t)

	points := []string{"/mnt/a", "/mnt/b"}
	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, time.Second, false)

	// Initially all false → IsHealthy should be false.
	if w.IsHealthy() {
		t.Fatalf("expected IsHealthy() to be false initially")
	}

	// Mark first mountpoint healthy.
	w.setHealthy("/mnt/a", true)

	if h, ok := w.IsMountHealthy("/mnt/a"); !ok || !h {
		t.Errorf("expected /mnt/a to be healthy after setHealthy")
	}
	if w.IsHealthy() {
		t.Errorf("expected IsHealthy() to be false when only one mountpoint is healthy")
	}

	// Mark second mountpoint healthy.
	w.setHealthy("/mnt/b", true)

	if h, ok := w.IsMountHealthy("/mnt/b"); !ok || !h {
		t.Errorf("expected /mnt/b to be healthy after setHealthy")
	}
	if !w.IsHealthy() {
		t.Errorf("expected IsHealthy() to be true when all mountpoints are healthy")
	}
}

func TestCheckMountedDirectoryDoesNotExist(t *testing.T) {
	resetPrometheusRegistry(t)

	// Use a clearly non-existent path
	nonexistent := "/this/path/should/not/exist/for_nfs_watchdog_test"
	points := []string{nonexistent}

	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, time.Second, false)

	err := w.checkMounted(nonexistent)
	if err == nil {
		t.Fatalf("expected error from checkMounted on non-existent directory, got nil")
	}
}

func TestWriteTestCreatesAndRemovesFile(t *testing.T) {
	resetPrometheusRegistry(t)

	tmpDir := t.TempDir()
	points := []string{tmpDir}

	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, time.Second, true)

	// We call writeTest directly (same package) to avoid isOnNFS dependency.
	if err := w.writeTest(tmpDir); err != nil {
		t.Fatalf("writeTest failed in temp dir: %v", err)
	}

	// Ensure no leftover test files remain.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) failed: %v", tmpDir, err)
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".nfs_mounter_test_") {
			t.Errorf("found leftover test file %q after writeTest", filepath.Join(tmpDir, e.Name()))
		}
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	resetPrometheusRegistry(t)

	tmpDir := t.TempDir()
	points := []string{tmpDir}

	w := NewWatchdog("test-program", "1.0.0", "test_ns", points, 10*time.Millisecond, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Give it a tiny bit of time to start, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Start did not return after context cancel")
	}
}
