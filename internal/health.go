package internal

import (
	"net/http"
	"strings"
)

type HealthHandlers struct {
	watchdog           *Watchdog
	healthPath         string
	mountPointsSubpath string
}

func NewHealthHandler(watchdog *Watchdog, healthPath, mountPointsSubpath string) *HealthHandlers {
	return &HealthHandlers{watchdog, healthPath, mountPointsSubpath}
}

func (s *HealthHandlers) HandleMountPoints(w http.ResponseWriter, r *http.Request) {
	prefix := s.healthPath + "/" + s.mountPointsSubpath
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}

	raw := strings.TrimPrefix(r.URL.Path, prefix)
	if raw == "" {
		http.Error(w, "mount point path required", http.StatusBadRequest)
		return
	}

	// Ensure leading slash: "var/vcap/store/dir" -> "/var/vcap/store/dir"
	mp := "/" + strings.TrimPrefix(raw, "/")

	healthy, ok := s.watchdog.IsMountHealthy(mp)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if healthy {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unhealthy\n"))
	}
}

func (s *HealthHandlers) HandleMain(w http.ResponseWriter, _ *http.Request) {
	if s.watchdog.IsHealthy() {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unhealthy\n"))
	}

}
