package internal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Watchdog struct {
	mountPoints          []string
	checkInterval        time.Duration
	enableWriteTest      bool
	mu                   sync.RWMutex
	lastHealthy          map[string]bool
	buildInfo            *prometheus.GaugeVec
	nfsMountHealthy      *prometheus.GaugeVec
	nfsChecksTotal       *prometheus.CounterVec
	nfsRemountsTotal     *prometheus.CounterVec
	nfsWriteTestDuration *prometheus.HistogramVec
}

func NewWatchdog(programName, programVersion, namespace string, points []string, interval time.Duration, enableWriteTest bool) *Watchdog {
	// Build info metric

	var writeTestMetric *prometheus.HistogramVec

	if enableWriteTest {
		writeTestMetric = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "write_test_duration_seconds",
				Help:      "Duration of NFS mount write test",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"mountpoint"},
		)
	}
	m := &Watchdog{
		mountPoints:     points,
		checkInterval:   interval,
		enableWriteTest: enableWriteTest,
		lastHealthy:     make(map[string]bool, len(points)),

		buildInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_info",
				Help:      "Build information for " + programName,
			},
			[]string{"program", "version"},
		),
		nfsMountHealthy: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "mount_healthy",
				Help:      "1 if NFS mount is healthy, 0 otherwise",
			},
			[]string{"mountpoint"},
		),

		nfsChecksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "checks_total",
				Help:      "Number of NFS health checks",
			},
			[]string{"mountpoint", "result"},
		),

		nfsRemountsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "remounts_total",
				Help:      "Number of NFS remount attempts (reserved for future self-healing)",
			},
			[]string{"mountpoint", "result"},
		),

		nfsWriteTestDuration: writeTestMetric,
	}

	m.buildInfo.WithLabelValues(programName, programVersion).Set(1)

	// Initialize lastHealthy default to false
	for _, mp := range points {
		m.lastHealthy[mp] = false
	}

	return m
}

func (m *Watchdog) setHealthy(mountPoint string, healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastHealthy[mountPoint] = healthy
}

func (m *Watchdog) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, mp := range m.mountPoints {
		if !m.lastHealthy[mp] {
			return false
		}
	}
	return true
}

func (m *Watchdog) IsMountHealthy(mountPoint string) (bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.lastHealthy[mountPoint]
	return h, ok
}

func (m *Watchdog) CheckMountPoint(mountPoint string) {
	err := m.checkMounted(mountPoint)
	if err != nil {
		m.nfsChecksTotal.WithLabelValues(mountPoint, "error").Inc()
		m.nfsMountHealthy.WithLabelValues(mountPoint).Set(0)
		m.setHealthy(mountPoint, false)
		log.Printf("mountpoint %s unhealthy: %v", mountPoint, err)
	} else {
		m.nfsChecksTotal.WithLabelValues(mountPoint, "ok").Inc()
		m.nfsMountHealthy.WithLabelValues(mountPoint).Set(1)
		m.setHealthy(mountPoint, true)
	}
}

func (m *Watchdog) CheckAll() {
	for _, mp := range m.mountPoints {
		m.CheckMountPoint(mp)
	}
}

func (m *Watchdog) checkMounted(mountPoint string) error {
	// Check directory exists
	info, err := os.Stat(mountPoint)
	if err != nil {
		return fmt.Errorf("stat(%s) failed: %w", mountPoint, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", mountPoint)
	}

	// Check /proc/mounts for NFS
	isNFS, err := m.isOnNFS(mountPoint)
	if err != nil {
		return fmt.Errorf("checking /proc/mounts failed: %w", err)
	}
	if !isNFS {
		return fmt.Errorf("%s is not an NFS mount", mountPoint)
	}

	// Write test
	if m.enableWriteTest {
		if err := m.writeTest(mountPoint); err != nil {
			return fmt.Errorf("write test failed on %s: %w", mountPoint, err)
		}
	}
	return nil
}

func (m *Watchdog) isOnNFS(mountPoint string) (bool, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		mp := fields[1]
		fsType := fields[2]

		// /proc/mounts uses escaped paths, but for simple BOSH paths
		// without spaces, a direct comparison is fine.
		if mp == mountPoint && (fsType == "nfs" || strings.HasPrefix(fsType, "nfs4")) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, errors.New("mount-point not found in /proc/mounts")
}

func (m *Watchdog) writeTest(mountPoint string) error {
	timer := prometheus.NewTimer(m.nfsWriteTestDuration.WithLabelValues(mountPoint))
	defer timer.ObserveDuration()

	name := fmt.Sprintf(".nfs_mounter_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	path := filepath.Join(mountPoint, name)

	if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return nil
}

func (m *Watchdog) Start(ctx context.Context) {
	log.Printf("starting watchdog, interval=%s, mountpoints=%v", m.checkInterval, m.mountPoints)

	// Initial check so /health reflects state quickly
	m.CheckAll()

	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("watchdog received context cancellation, stopping")
			return
		case <-ticker.C:
			m.CheckAll()
		}
	}
}
