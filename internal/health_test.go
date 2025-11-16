package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helper to build a minimal watchdog without touching Prometheus
func newTestWatchdog(mountPoints []string, healthyMap map[string]bool) *Watchdog {
	return &Watchdog{
		mountPoints: mountPoints,
		lastHealthy: healthyMap,
	}
}

func TestHandleMain_Healthy(t *testing.T) {
	watchdog := newTestWatchdog(
		[]string{"/mnt/a", "/mnt/b"},
		map[string]bool{
			"/mnt/a": true,
			"/mnt/b": true,
		},
	)

	h := NewHealthHandler(watchdog, "/health", "mount-points")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HandleMain(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != "ok\n" {
		t.Fatalf("expected body %q, got %q", "ok\n", string(body))
	}
}

func TestHandleMain_Unhealthy(t *testing.T) {
	watchdog := newTestWatchdog(
		[]string{"/mnt/a", "/mnt/b"},
		map[string]bool{
			"/mnt/a": true,
			"/mnt/b": false,
		},
	)

	h := NewHealthHandler(watchdog, "/health", "mount-points")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HandleMain(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != "unhealthy\n" {
		t.Fatalf("expected body %q, got %q", "unhealthy\n", string(body))
	}
}

func TestHandleMountPoints_WrongPrefix(t *testing.T) {
	watchdog := newTestWatchdog(nil, map[string]bool{})
	h := NewHealthHandler(watchdog, "/health", "mount-points")

	// URL does not start with /health/mount-points
	req := httptest.NewRequest(http.MethodGet, "/something-else/var/vcap/store", nil)
	rec := httptest.NewRecorder()

	h.HandleMountPoints(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.StatusCode)
	}
}

func TestHandleMountPoints_MissingMountPath(t *testing.T) {
	watchdog := newTestWatchdog(nil, map[string]bool{})
	h := NewHealthHandler(watchdog, "/health", "mount-points")

	// Exactly the prefix: /health/mount-points
	req := httptest.NewRequest(http.MethodGet, "/health/mount-points", nil)
	rec := httptest.NewRecorder()

	h.HandleMountPoints(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "mount point path required") {
		t.Fatalf("expected error message about missing mount point, got %q", string(body))
	}
}

func TestHandleMountPoints_UnknownMountPoint(t *testing.T) {
	watchdog := newTestWatchdog(
		[]string{"/var/vcap/store/proftpd"},
		map[string]bool{
			"/var/vcap/store/proftpd": true,
		},
	)
	h := NewHealthHandler(watchdog, "/health", "mount-points")

	// Path refers to a mountpoint that watchdog does NOT know about
	req := httptest.NewRequest(http.MethodGet, "/health/mount-points/var/vcap/store/unknown", nil)
	rec := httptest.NewRecorder()

	h.HandleMountPoints(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.StatusCode)
	}
}

func TestHandleMountPoints_HealthyMount(t *testing.T) {
	mp := "/var/vcap/store/proftpd"

	watchdog := newTestWatchdog(
		[]string{mp},
		map[string]bool{
			mp: true,
		},
	)
	h := NewHealthHandler(watchdog, "/health", "mount-points")

	// URL: /health/mount-points/var/vcap/store/proftpd → mp = "/var/vcap/store/proftpd"
	req := httptest.NewRequest(http.MethodGet, "/health/mount-points/var/vcap/store/proftpd", nil)
	rec := httptest.NewRecorder()

	h.HandleMountPoints(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != "ok\n" {
		t.Fatalf("expected body %q, got %q", "ok\n", string(body))
	}
}

func TestHandleMountPoints_UnhealthyMount(t *testing.T) {
	mp := "/var/vcap/store/proftpd"

	watchdog := newTestWatchdog(
		[]string{mp},
		map[string]bool{
			mp: false,
		},
	)
	h := NewHealthHandler(watchdog, "/health", "mount-points")

	req := httptest.NewRequest(http.MethodGet, "/health/mount-points/var/vcap/store/proftpd", nil)
	rec := httptest.NewRecorder()

	h.HandleMountPoints(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != "unhealthy\n" {
		t.Fatalf("expected body %q, got %q", "unhealthy\n", string(body))
	}
}
