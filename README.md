# nfs_mounter_agent

`nfs_mounter_agent` is a small Go daemon that monitors one or more NFS mount points and exposes:

* Prometheus metrics
* HTTP health endpoints (global and per-mount-point)

It is designed for environments where other services depend on NFS being mounted and writable — for example, BOSH jobs, containers, or VM-based workloads.

The agent performs:

* Directory existence check
* NFS filesystem type check (`/proc/mounts`)
* Optional write/delete test
* Metrics reporting and periodic health evaluation

It **does not** perform remounting itself; the goal is monitoring and signaling, not automatic repair.

## Features

* Monitors multiple mount points (`--mount-point` repeated flag)
* Global `/health` endpoint
* Per-mount health: `/health/mount-points/<path>`
* Prometheus `/metrics` endpoint
* Optional write test (`--enable-write-test`)
* Small, simple, no dependencies outside the Go standard library and Prometheus client

## Example usage

```bash
./nfs_mounter_agent \
  --mount-point /var/vcap/store/proftpd \
  --mount-point /data/shared \
  --listen-address 0.0.0.0:9090 \
  --enable-write-test \
  --check-interval 10s
```

## HTTP endpoints

### `/metrics`

Prometheus metrics include:

* `nfsma_build_info`
* `nfsma_mount_healthy`
* `nfsma_checks_total`
* `nfsma_write_test_duration_seconds` (if enabled)

### `/health`

Returns:

* `200 OK` if **all** mount points are healthy
* `503 Service Unavailable` otherwise

### `/health/mount-points/<path>`

Per-mount health.
Example:

```
/health/mount-points/var/vcap/store/job
```

Maps to the mount point:

```
/var/vcap/store/job
```

## Flags

```
--listen-address       Address for HTTP server (default: 0.0.0.0:9090)
--mount-point          Mount point to monitor (repeatable, absolute path)
--check-interval       Interval between checks (default: 30s)
--enable-write-test    Enable write/delete test in mount health checks
--health-path          Base health path (default: /health)
--telemetry-path       Metrics endpoint path (default: /metrics)
--telemetry-namespace  Metric namespace
```

## Build

```bash
./build.sh
```

## Tests

```bash
./test.sh
```

## License

[MIT](LICENSE)
