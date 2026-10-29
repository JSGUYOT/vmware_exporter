## 2.0.0 / 2026-10-29

* [CHANGE] `vmware-exporter` is renamed `vmware_exporter` everywhere: binary, systemd unit
  (`vmware_exporter.service`), system user and group, configuration directory
  (`/etc/vmware_exporter`, also the default `-config.file`), environment file, container
  image, alert group and dashboard tag. Existing installations must be migrated (see README).
* [CHANGE] Native labels renamed: `host` → `host_name`, `cluster` → `cluster_name`,
  `datacenter` → `dc_name`, `datastore` → `ds_name`, `vm` → `vm_name`. Queries, alerts and
  dashboards built on the old names must be updated (the shipped ones are). `host` is no
  longer reserved, so it can be set through `extra_labels` or relabeling.
* [CHANGE] `VMwareExporterSlowCollection` now fires when a collection uses more than 80% of its
  timeout (was a fixed 50 s, wrong for intervals other than 60 s).
* [FEATURE] `folder_name` label (VM folder) on every native VM series.
* [FEATURE] Five linked Grafana dashboards: overview, clusters, ESX hosts, VMs, datastores, with
  drill-down links (cluster → host → VM, datastore names → datastores). The datastores dashboard
  shows capacity, thin provisioning, accessibility, datastore clusters, latency / throughput /
  IOPS per datastore and per ESX host. The overview's uid changed from
  `vmware-exporter-overview` to `vmware-overview`: delete the old dashboard after importing.
  Screenshots in the README.
* [FEATURE] `vmware_collection_interval_seconds` and `vmware_collection_timeout_seconds`
  (configured interval / timeout per vCenter).
* [FEATURE] `collectors.exporter` (default `true`): set it to `false` to drop the collection's
  own metrics (`vmware_collection_*`, `vmware_collector_*`, `vmware_exporter_*` properties and
  series counters); `vmware_up` is always emitted.
* [ENHANCEMENT] ESX counters collected by default: `datastore.read`, `datastore.write`,
  `datastore.numberReadAveraged`, `datastore.numberWriteAveraged` (per datastore).
* [ENHANCEMENT] Overview dashboard: "Collection per vCenter" section (last / max duration,
  duration / timeout, data age, collections and failures, duration per collector).

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
