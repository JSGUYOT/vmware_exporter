## main / unreleased

## 1.0.0 / 2026-10-16

First release.

* [FEATURE] Prometheus exporter for vCenter 8.x and 9.x (VCF 9 / VVF 9) with automatic API
  version negotiation: vCenter, alarms, datacenters, clusters (HA, DRS), ESX hosts (hardware
  sensors, multipath, uplinks, services, certificates), VMs, datastores, vDS, resource pools,
  vSAN, vCenter appliance, real-time performance counters.
* [FEATURE] Multi-vCenter with background collection and a cache: a scrape never calls vCenter.
* [FEATURE] Select vCenters per scrape: one endpoint per vCenter (`/metrics/<name|host>`),
  `/metrics?target=` and the pryorda-compatible `/metrics?vsphere_host=`.
* [FEATURE] Optional pryorda/vmware_exporter compatibility (`collectors: {pryorda: true}`):
  same metric names, labels, units and performance counters, so existing dashboards keep
  working; pryorda's 5 alerting rules shipped. Built from the same vCenter reads as the
  native series (no extra load on vCenter).
* [FEATURE] Optional protection of the HTTP endpoints (`web_config_file`, exporter-toolkit
  format: basic authentication and/or TLS); public by default.
* [FEATURE] 76 alerting rules with promtool tests and a Grafana dashboard.
* [FEATURE] Build from `vendor/` (offline, default) or from the Go module proxy
  (`make build DEPS=online`); promu support (`.promu.yml`, `make release-tarballs`).
* [ENHANCEMENT] Release archives for Linux amd64/arm64 and Windows amd64 with SHA-256 checksums.
