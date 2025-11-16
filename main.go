package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"nfs_mounter_agent/internal"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var ProgramVersion = "dev"

const (
	programName        = "nfs_mounter_agent"
	mountPointsSubpath = "mount-points/"
)

// MountPoints implements flag.Value to allow --mount-point repeated.
type MountPoints []string

func (m *MountPoints) String() string {
	return strings.Join(*m, ",")
}

func (m *MountPoints) Set(value string) error {
	if !filepath.IsAbs(value) {
		return fmt.Errorf("mount point must be an absolute path: %q", value)
	}
	*m = append(*m, value)
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	listenAddressPtr := flag.String("listen-address", "0.0.0.0:9090", "Listen address for HTTP server")
	telemetryPathPtr := flag.String("telemetry-path", "/metrics", "Telemetry path")
	namespacePtr := flag.String("telemetry-namespace", "nfsma", "Metrics namespace")
	healthPathPtr := flag.String("health-path", "/health", "Health check path (global and per mount-point sub-path: '"+mountPointsSubpath+"')")
	checkIntervalPtr := flag.Duration("check-interval", 30*time.Second, "Interval between mount checks")
	enableWriteTestPtr := flag.Bool("enable-write-test", false, "Enable write-test as part of the mount health check")

	var mountPoints MountPoints
	flag.Var(&mountPoints, "mount-point", "Mount point to monitor (can be repeated, absolute paths only)")

	flag.Parse()

	if len(mountPoints) == 0 {
		log.Fatal("no mount points configured (use --mount-point /path/to/mount)")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchdog := internal.NewWatchdog(programName, ProgramVersion, *namespacePtr, mountPoints, *checkIntervalPtr, *enableWriteTestPtr)
	healthHandler := internal.NewHealthHandler(watchdog, *healthPathPtr, mountPointsSubpath)

	go watchdog.Start(ctx)

	// HTTP handlers
	http.Handle(*telemetryPathPtr, promhttp.Handler())

	// Global health: all mount points must be healthy
	http.HandleFunc(*healthPathPtr, healthHandler.HandleMain)

	// Per-mount health: /health/mount-points/var/vcap/store/dir -> /var/vcap/store/dir
	http.HandleFunc(*healthPathPtr+"/mount-points/", healthHandler.HandleMountPoints)

	log.Printf("Starting %s v%s on %s (metrics: %s, health: %s, per-mount health base: %s/%s...)",
		programName, ProgramVersion, *listenAddressPtr, *telemetryPathPtr, *healthPathPtr, *healthPathPtr, mountPointsSubpath)

	if err := http.ListenAndServe(*listenAddressPtr, nil); err != nil {
		log.Fatalf("cannot start server: %v", err)
	}
}
