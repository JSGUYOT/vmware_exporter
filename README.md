# vmware_exporter

[![CI](https://github.com/JSGUYOT/vmware_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/JSGUYOT/vmware_exporter/actions/workflows/ci.yml)

> [!WARNING]
> **Project under active development, not yet battle-tested.** The code has not yet been
> thoroughly tested on real infrastructure and may contain bugs.
> It is not recommended for production: use at your own risk, preferably
> in a test environment, with a **read-only** vCenter account.
> Behaviour, configuration and metrics may still change without notice.

Prometheus exporter written in Go for **vCenter 8.x** and **vCenter 9.x (VCF 9 / VVF 9)**.
It exposes the full state of the vSphere infrastructure — resources, health, alarms —
for dashboards (Grafana) **and** alerting (Prometheus / Alertmanager).

- Go 1.25, **govmomi v0.56.0** (vim25 API 9.1), client_golang 1.23
- Single static binary, no system dependencies, 100 % offline build (`vendor/`)
- Multi-vCenter (8 and 9 side by side), background collection with a cache: a Prometheus scrape never triggers a call to vCenter
- Ships with 76 tested alerting rules and a Grafana dashboard
- Optional [pryorda/vmware_exporter compatibility](#migrating-from-pryordavmware_exporter): same metric names and labels, so existing dashboards keep working

## Why a new exporter

| | pryorda/vmware_exporter | prezhdarov/vmware-exporter | **vmware_exporter** |
|---|---|---|---|
| Language | Python (pyVmomi) | Go | Go (govmomi 0.56) |
| Maintained / VCF 9 | no / no | partial | yes, 8/9 version negotiation |
| Scope | VM, ESX | resources (perf) | VM, ESX, clusters, datastores, vDS, resource pools, vSAN, vCenter appliance |
| vCenter alarms | alarm counts per VM/host/datastore (optional) | no | **yes** (all entities, a single request) |
| Hardware health (IPMI), multipath, uplinks, certificates | no | no | **yes** |
| Bundled alerting rules | 5 (VM memory, snapshots) | no | **76 rules + promtool tests**, plus pryorda's 5 |
| Reuse pryorda dashboards | — | no | **yes** (optional `pryorda` collector) |

## vCenter 8 / vCenter 9 compatibility: what is handled

Differences between the two release lines are handled explicitly:

1. **SOAP API version negotiation.** By default govmomi speaks the latest known API (9.1).
   The exporter reads `/sdk/vimServiceVersions.xml` on each vCenter and uses *its* version
   (8.0.x.0 or 9.x.0.0). The vSAN endpoint (`/sdk/vsanServiceVersions.xml`) is negotiated separately.
   The version in use is visible in `vmware_vcenter_info{api_version_used=...}`.
2. **Automatic fallback on unknown properties.** If a requested property does not exist in the
   vCenter's API version (`InvalidProperty` fault), it is dropped and the request is retried:
   collection degrades instead of failing. Counter: `vmware_exporter_unsupported_properties`.
3. **No dependency on features removed or deprecated in 9.x**: vCLS (deprecated in 9.0,
   disabled in 9.1), SIOC and Storage DRS load balancer (removed in 9.0), Host Profiles (deprecated),
   vLCM baselines (removed), Enhanced Linked Mode (deprecated). System VMs (vCLS, Supervisor,
   EAM agents) are identified by `managed_by` and can be excluded (`exclude_system_vms`).
4. **Modern REST API only** (`/api/...`, available since 7.0U2) — never the legacy `/rest/...`
   prefix, which is going away. Dedicated REST session, reused and renewed on 401.
5. **TLS 1.3** (the vCenter 9 default) is supported; `tls_min_version: "1.3"` can be set.
6. **Native multi-vCenter**: with ELM dropped in VCF 9, each vCenter in the fleet is
   an independent target (`targets:`), labelled with `vcenter` and your `extra_labels`.
7. **No parsing of product names** (ESXi → ESX in 9): only numeric versions are read.

The automated tests replay a full collection against the govmomi simulator advertising
**8.0.3.0, 9.0.0.0 and 9.1.0.0** in turn (see "Tests").

## What is monitored

| Area | Example metrics | Typical alerts |
|---|---|---|
| Exporter | `vmware_up`, duration, failed collectors | vCenter unreachable, stale data |
| vCenter | version, TLS certificate, sessions, inventory | certificate < 30 days |
| **vCenter alarms** | `vmware_alarm_triggered{alarm,entity,entity_type,status,acknowledged}` | any unacknowledged red/yellow alarm |
| Appliance (VAMI) | system/memory/storage/database health, services, backups | service stopped, backup failed or > 26 h old |
| Clusters | CPU/memory capacity and demand, HA, DRS, DRS score, HA failover | HA disabled, failover capacity lost, resource pressure |
| ESX | connection, maintenance, quarantine, HA agent state, IPMI sensors, hardware components, storage paths, uplinks, SSH/NTP, certificate, reboot required | ESX not responding, red sensor, LUN without path, uplink down |
| VM | power, heartbeat, tools, pending question, consolidation, snapshots (count, age), guest disks, HA protection, ballooning/swap | heartbeat lost, snapshot > 3 days, guest disk < 5 %, CPU ready > 10 % |
| Datastores | capacity, free, provisioned, accessibility per ESX, maintenance | > 85/95 %, full within 7 days, inaccessible |
| Network | vDS (per-ESX synchronization), portgroups | ESX outOfSync |
| vSAN | overall health, per group, per test | red health, failed test |
| Real-time performance | CPU ready/co-stop/latency, disk/datastore latency, network, packet drops, power | datastore latency > 30 ms, network drops |

Full reference: [docs/METRICS.md](docs/METRICS.md).
Status convention: `0 = gray/unknown, 1 = green, 2 = yellow, 3 = red`.

Inventory labels: `dc_name`, `cluster_name`, `host_name`, `ds_name`, `vm_name` (the same names as
pryorda/vmware_exporter).

VM series are identified by `vcenter`, `dc_name`, `folder_name`, `vm_name` and `vm_moid` only
(`folder_name` = name of the VM's folder, `""` at the datacenter root VM folder or in a vApp):
`host_name` and `cluster_name` change on every vMotion and would break graph continuity and
alert `for` durations. They are carried by `vmware_vm_info`, to be joined when needed:
`vmware_vm_cpu_usage_mhz * on(vcenter, vm_moid) group_left(cluster_name, host_name) vmware_vm_info`.

## Architecture

```
            ┌──────────── vmware_exporter ────────────┐
vCenter 8 ◄─┤ collection loop (interval)               │
vCenter 9 ◄─┤   1. connect / resume session            │      GET /metrics
   ...      │   2. ContainerView + PropertyCollector   ├──►  (cache, instant)     ◄── Prometheus
            │   3. collectors in parallel              │
            │   4. perf counters (QueryPerf, batches)  │
            └──────────────────────────────────────────┘
```

One collection = one ContainerView over the inventory, then one PropertyCollector request per
object type (targeted properties, pagination handled): a few dozen calls regardless of
the number of VMs. Real-time counters are requested in batches (`batch_size`) with bounded
parallelism (`max_concurrency`). Alarms are read in **a single request**
(`triggeredAlarmState` of the root folder, propagated from all entities).

## Installation

Dependencies can come from the repository or from the Internet, chosen at build time with `DEPS`:

```bash
# On a machine with Go >= 1.25
make build                 # -> dist/vmware_exporter (Linux amd64, static), from vendor/: no network needed
make build DEPS=online     # same, modules downloaded from the Go proxy and checked against go.sum
make build-windows         # -> dist/vmware_exporter.exe (DEPS works the same way)
```

- `DEPS=vendor` (default) builds with `-mod=vendor`: everything comes from `vendor/`, which
  suits air-gapped environments.
- `DEPS=online` builds with `-mod=readonly`: `vendor/` is ignored and modules are downloaded
  from `proxy.golang.org` (or `$GOPROXY`); `go.mod` / `go.sum` are never modified.
- Without make: `go build -mod=vendor ./cmd/vmware_exporter` or `go build -mod=readonly ./cmd/vmware_exporter`.
- After changing dependencies, `make vendor` refreshes `vendor/` (needs network access).

The exporter can also be built with [promu](https://github.com/prometheus/promu), the build tool
of the official Prometheus exporters, configured in [.promu.yml](.promu.yml); the version comes
from the `VERSION` file:

```bash
promu build                          # ./vmware_exporter, from vendor/ (no network needed)
GOFLAGS=-mod=readonly promu build    # modules downloaded from the Go proxy
promu tarball --prefix dist .        # dist/vmware_exporter-<version>.linux-amd64.tar.gz
promu crossbuild                     # linux/amd64, linux/arm64, windows/amd64 (needs Docker)
make promu [DEPS=online]             # same as the first two, through make
make promu-tarball                   # build + release tarball in dist/
```

`make build` produces the static Linux amd64 binary in `dist/` (not tracked in the repository).
Prebuilt archives are attached to each [GitHub release](https://github.com/JSGUYOT/vmware_exporter/releases).

```bash
# Deployment
useradd --system --no-create-home --shell /usr/sbin/nologin vmware-exporter
install -m 0755 dist/vmware_exporter /usr/local/bin/
install -d -m 0750 -o root -g vmware-exporter /etc/vmware-exporter
install -m 0640 -o root -g vmware-exporter config.example.yml /etc/vmware-exporter/config.yml
# Service account password (never in the YAML)
printf '%s' 'ThePassword' > /etc/vmware-exporter/vcenter.pass
chown root:vmware-exporter /etc/vmware-exporter/vcenter.pass && chmod 0640 /etc/vmware-exporter/vcenter.pass
echo 'VMWARE_EXPORTER_USER=svc-prometheus@vsphere.local' > /etc/vmware-exporter/vmware-exporter.env
# vCenter CA (VMCA: https://<vcenter>/certs/download.zip) or your own PKI
cp ca.pem /etc/vmware-exporter/ca.pem

vmware_exporter -config.file /etc/vmware-exporter/config.yml -config.check
install -m 0644 deploy/systemd/vmware-exporter.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now vmware-exporter
curl -s localhost:9272/metrics | grep '^vmware_up'
```

Container image (optional): `docker build -t vmware-exporter:$(cat VERSION) .` (built from `vendor/`;
add `--build-arg GO_MOD=readonly` to download modules instead).

## Service account and permissions

Create a dedicated account and grant it the **Read-only** role, propagated from the root of
the inventory (vCenter > Permissions). This covers inventory, performance, alarms and vSAN health.

- **VCF 9 with identity federation** (VCF Identity Broker, ADFS, Entra ID, Okta…): use a
  **local SSO** account (`…@vsphere.local`), since SOAP password authentication does not
  accept federated accounts.
- `vmware_vcenter_sessions` only appears if the account is allowed to list sessions
  (*Sessions.TerminateSession* privilege) — optional.
- `appliance` collector (`/api/appliance/...` API): depending on the version, some endpoints
  may require an additional role on the appliance side. A denied or missing endpoint yields
  `vmware_appliance_endpoint_up{endpoint=...} 0` without blocking the rest of the collection.

## Configuration

See [config.example.yml](config.example.yml) (fully commented). Key points:

- `defaults:` is inherited by every target; everything can be overridden per target.
- Secrets: `password_file` (recommended) or `password_env`; `${VAR}` is interpolated everywhere.
- `interval` (default 60 s, minimum 20 s) and `timeout` (default `interval - 5s`).
- `collectors:` enables/disables each family; `filters:` restricts by regex.
- `perf.host_counters` / `perf.vm_counters`: vSphere counters as `group.counter.rollup`,
  with a `:*` suffix for all instances (e.g. `datastore.totalReadLatency.average:*`).
- `extra_labels:` adds static labels (site, env…) to every series.

Counters collected by default:

- **ESX**: cpu.usage/utilization/coreUtilization, mem.usage/active/consumed/vmmemctl/swapinRate/swapoutRate,
  disk.maxTotalLatency/read/write/commandsAborted/busResets, net.received/transmitted/droppedRx/droppedTx/errorsRx/errorsTx,
  power.power, datastore.totalReadLatency/totalWriteLatency (per datastore)
- **VM**: cpu.usage/ready/costop/latency/readiness, mem.usage/active/vmmemctl/swapped/swapinRate,
  disk.maxTotalLatency/read/write, net.received/transmitted/droppedRx/droppedTx

Prometheus name: `vmware_<host|vm>_perf_<group>_<counter>_<rollup>_<unit>` (percentages as 0–100,
bytes and seconds in base units), label `perf_instance` (`""` = aggregate).
Example: CPU ready in % per vCPU = `vmware_vm_perf_cpu_ready_summation_seconds{perf_instance=""} / 20 / ignoring(perf_instance) vmware_vm_vcpus`.

## HTTP endpoints

| Path | Purpose |
|---|---|
| `/metrics` | metrics (cache of the last collection from each vCenter) |
| `/metrics/<name\|host>` | one endpoint per vCenter, e.g. `/metrics/vc8-site-a` (configured `name` or host name of its `url`) |
| `/metrics?target=<name\|host>` | only the given vCenter(s): configured `name` or host name of its `url`; repeatable or comma-separated |
| `/metrics?vsphere_host=<host>` | same, with the parameter name used by pryorda/vmware_exporter |
| `/healthz` | process liveness (protected too when `web_config_file` is set) |
| `/` | landing page (list of targets, with a link to each vCenter endpoint) |

### Scraping a single vCenter

By default one scrape of `/metrics` returns every configured vCenter. To scrape them
separately (one job per vCenter, different intervals, or only some of them), either use the
per-vCenter endpoint `/metrics/<name>`, or pass `?target=` — the multi-target pattern of
blackbox_exporter. Only vCenters declared in
the configuration can be selected (credentials stay on the exporter); any other value
returns HTTP 404. A filtered response contains only vCenter series, not the exporter's
own `go_*` / `process_*` metrics. Collection still runs in the background, so filtering
never triggers a call to vCenter.

With one endpoint per vCenter, each job just sets its `metrics_path`:

```yaml
- job_name: vmware-vc8-site-a
  metrics_path: /metrics/vc8-site-a
  static_configs:
    - targets: ['exporter-host.example.local:9272']
```

With `?target=`, one job covers several vCenters through relabeling:

```yaml
- job_name: vmware-per-vcenter
  static_configs:
    - targets: [vc8-site-a, vcf9-mgmt]
  relabel_configs:
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: exporter-host.example.local:9272
```

Do not combine this with an unfiltered job on the same exporter, or every series is
ingested twice. Full snippet: [deploy/prometheus/prometheus-scrape.yml](deploy/prometheus/prometheus-scrape.yml).

## Securing the endpoints

By default the HTTP endpoints are public: anyone reaching port 9272 can read the inventory
(VM and host names, versions, alarms), and the exporter logs a warning at startup. To protect
them, set `web_config_file` in the configuration. It points to a standard Prometheus web
configuration file ([exporter-toolkit](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md)
format, as for node_exporter), which enables basic authentication and/or HTTPS:

```yaml
# /etc/vmware-exporter/config.yml
web_config_file: /etc/vmware-exporter/web-config.yml
```

```yaml
# /etc/vmware-exporter/web-config.yml  (example: deploy/web-config.example.yml)
tls_server_config:                     # optional
  cert_file: /etc/vmware-exporter/tls/exporter.crt
  key_file: /etc/vmware-exporter/tls/exporter.key
basic_auth_users:
  prometheus: $2y$10$...               # bcrypt hash: htpasswd -nBC 10 "" | tr -d ':\n'
```

- Every endpoint is then protected, `/healthz` included.
- The file is validated by `-config.check` and at startup.
- `web_tls` (HTTPS without authentication) remains available; it cannot be combined with
  `web_config_file`.

On the Prometheus side, add the credentials to the scrape job:

```yaml
- job_name: vmware
  basic_auth:
    username: prometheus
    password_file: /etc/prometheus/vmware-exporter.pass
  static_configs:
    - targets: ['exporter-host.example.local:9272']
```

## Alerting

[deploy/prometheus/vmware-alerts.yml](deploy/prometheus/vmware-alerts.yml): 76 rules in 8 groups
(exporter, vCenter + appliance, cluster, ESX + performance, VM, datastore, network, vSAN).

- **vCenter alarm relay**: `VMwareVCenterAlarmCritical` / `VMwareVCenterAlarmWarning`
  turn every active, unacknowledged native alarm into a Prometheus alert (label `source=vcenter`).
  Everything vCenter can detect thus reaches Alertmanager without redefining anything;
  acknowledging the alarm in the vSphere Client clears the alert.
- **Prometheus-specific rules** for what vCenter alarms cover poorly: trends
  (datastore full within 7 days), redundancy (multipath, uplinks), CPU ready, old snapshots,
  vCenter backups, failed collections…
- Severities: `critical`, `warning`, `info`. Thresholds are starting values to be tuned.

```bash
promtool check rules deploy/prometheus/vmware-alerts.yml deploy/prometheus/pryorda-alerts.yml
cd deploy/prometheus && promtool test rules vmware-alerts.test.yml pryorda-alerts.test.yml
```

Prometheus configuration snippet: [deploy/prometheus/prometheus-scrape.yml](deploy/prometheus/prometheus-scrape.yml).

## Grafana

Four linked dashboards (Grafana >= 10, no plugin), in `deploy/grafana/`:

| File | Dashboard | Content |
|---|---|---|
| `vmware-overview.json` | Overview (uid `vmware-overview`) | vCenters, active alarms, health counters, clusters, top ESX / VMs, datastores, vSAN, exporter |
| `vmware-clusters.json` | Clusters (uid `vmware-clusters`) | status, CPU / memory capacity, N-1, HA / DRS, ESX hosts, resource pools, vSAN, alarms |
| `vmware-hosts.json` | ESX hosts (uid `vmware-hosts`) | status, hardware, CPU / memory, storage and datastore latency, NICs, sensors, services, VMs of the host |
| `vmware-vms.json` | VMs (uid `vmware-vms`) | placement, CPU (ready, co-stop), memory, disks, guest file systems, snapshots, network, alarms |

- Navigation: the header links (tag `vmware-exporter`) switch between the dashboards, keeping
  the time range and the variables.
- Drill-down: click a cluster, ESX host or VM name in a table or a graph to open it
  (overview → cluster → ESX host → VM); the VMs dashboard links back to the VM's ESX host and cluster.
- Variables: `datasource`, `vcenter`, `dc_name`, `cluster_name`, `host_name`, `folder_name`,
  `vm_name` (chained).
- Import: *Dashboards > New > Import*, upload each JSON file and choose your Prometheus data
  source (or `POST /api/dashboards/db`). Import all four so the links work. The overview's uid
  changed from `vmware-exporter-overview` to `vmware-overview`: delete the old dashboard.
- To modify them, edit `deploy/grafana/build_dashboard.py` and regenerate the four files with
  `python3 build_dashboard.py` (CI checks they are in sync).

## Migrating from pryorda/vmware_exporter

Dashboards and alerts built for [pryorda/vmware_exporter](https://github.com/pryorda/vmware_exporter)
can be reused as is. Enable the `pryorda` collector:

```yaml
defaults:
  collectors:
    pryorda: true
```

The exporter then also emits the pryorda series, next to the native ones:

- **Same names** (typos included: `vmware_datastore_provisoned_size`, `vmware_datastore_uncommited_size`),
  **same labels** (`vm_name`, `ds_name`, `host_name`, `dc_name`, `cluster_name`, `ds_cluster`,
  `partition`, `tools_status`, `version`, `build`, `hardware_model`…), **same units and values**
  (MB for memory, MHz for CPU, bytes for disks, sensor state 0=red…3=unknown, `n/a` / `unknown` defaults).
- **Same performance counters** (`vmware_vm_cpu_ready_summation`, `vmware_host_mem_swapinRate_average`…):
  latest real-time sample, raw vSphere units, as pryorda queries them. They need the `perf` collector
  (enabled by default).
- **Same scrape URL**: `/metrics?target=<host>` or `/metrics?vsphere_host=<host>` with the usual
  `__param_target` → `instance` relabeling (see [Scraping a single vCenter](#scraping-a-single-vcenter)).
  The vCenter must be declared in the configuration; pryorda's `section` parameter is not needed
  (credentials are per target) and is ignored.
- **pryorda's 5 alerting rules**, unchanged: [deploy/prometheus/pryorda-alerts.yml](deploy/prometheus/pryorda-alerts.yml).

Differences:

- Every series also carries `vcenter` (and `extra_labels`). Queries selecting on pryorda labels
  are unaffected; only `sum without(...)`-style aggregations may need the extra label.
- Not implemented: pryorda's optional `fetch_tags`, `fetch_custom_attributes` and `fetch_alarms`
  (all off by default in pryorda). For alarms, use the native `vmware_alarm_triggered`.
- `filters` (`vm_exclude`, `host_exclude`, `exclude_system_vms`…) also apply to pryorda series;
  templates are kept, as in pryorda (`vmware_vm_template`).
- pryorda's `esxi.json` dashboard queries names that pryorda itself never produced
  (`vmware_host_net_bytesRX_average`, `vmware_host_mem_usage_average`…): those panels stay empty.

The native series use the same label names as pryorda (`vm_name`, `host_name`, `cluster_name`,
`dc_name`, `ds_name`); the metric names and units differ (e.g. bytes instead of MB for memory), and native series
add a few labels of their own (`vm_moid`, `folder_name`, `datastore_moid`; pryorda's `ds_cluster`
is `datastore_cluster` natively).
To avoid any clash, the native metrics that had the same name as a pryorda metric are named:

| pryorda name (pryorda labels) | native name (native labels) |
|---|---|
| `vmware_vm_power_state` | `vmware_vm_power_status` |
| `vmware_vm_boot_timestamp_seconds` | `vmware_vm_boot_time_seconds` |
| `vmware_vm_snapshots` | `vmware_vm_snapshots_existing` |
| `vmware_host_power_state` | `vmware_host_power_status` |
| `vmware_host_connection_state` | `vmware_host_connection_status` |
| `vmware_host_maintenance_mode` | `vmware_host_in_maintenance` |
| `vmware_host_boot_timestamp_seconds` | `vmware_host_boot_time_seconds` |
| `vmware_datastore_accessible` | `vmware_datastore_is_accessible` |
| `vmware_datastore_maintenance_mode` | `vmware_datastore_in_maintenance` |
| `vmware_datastore_vms` | `vmware_datastore_vms_hosted` |

The pryorda series are built from the same vCenter reads and the same performance queries as
the native ones, so enabling them adds no load on vCenter; they do roughly double the number of
series for VMs, hosts and datastores in Prometheus. Enable the collector only where pryorda
dashboards are used.

## Tests

```bash
make test        # Go tests: 8.0.3 / 9.0 / 9.1 matrix on the simulator, session recovery,
                 # unreachable vCenter, simulated appliance REST API, metric name lint
make test-rules  # alerting rule unit tests (promtool), native and pryorda-compatible
make sim         # starts a simulated vCenter at https://127.0.0.1:8989/sdk (user / pass)
```

The simulator (`hack/vcsim`) lets you try the exporter, Prometheus and Grafana without any
infrastructure: `make sim VERSION_SIM=8.0.3 API_SIM=8.0.3.0`, then point a target at
`127.0.0.1:8989` (`username: user`, password `pass`, `insecure_skip_verify: true`).

## Known limitations and validation

- The automated tests use the govmomi simulator, not a real vCenter: validate first on
  an 8.x and a 9.x pre-production environment. At startup, watch the logs for unsupported-property
  and failed-collector warnings, as well as `vmware_exporter_unsupported_properties` (property missing
  from the API version) and `vmware_exporter_missing_properties` (property denied on some objects, e.g. permissions).
- Perf counters: average of the 20 s samples since the last collection, except for `latest`
  rollups (last sample) and `maximum`/`minimum` (extreme value, peaks are not smoothed).
- The exact format of `/api/appliance/recovery/backup/job/details` may vary by version;
  if it cannot be parsed, the backup metrics are simply absent.
- vSAN health is read from the health service cache (tests are not re-run): its
  freshness depends on the schedule configured on the vSAN side (`vmware_vsan_health_timestamp_seconds`).
- Out of scope (APIs separate from vCenter): SDDC Manager, NSX, VCF Operations, VCF Automation.

## Repository layout

```
cmd/vmware_exporter/     entry point (HTTP, registry, signals)
internal/config/         YAML loading / validation, secrets, defaults
internal/vsphere/        SOAP connection (version negotiation, session recovery), /api REST client
internal/collector/      collectors (vcenter, alarms, cluster, host, vm, datastore, network,
                         resourcepool, perf, vsan, appliance, pryorda) + cache + tests
deploy/prometheus/       alerting rules (native + pryorda-compatible), promtool tests, scrape snippet
deploy/grafana/          4 linked dashboards (JSON + generator)
deploy/systemd/          hardened systemd unit
deploy/web-config.example.yml  optional authentication / HTTPS for the endpoints
hack/vcsim/              simulated vCenter (8.x / 9.x) for testing
docs/METRICS.md          metrics reference
vendor/                  dependencies (offline build, make build; DEPS=online ignores it)
```

## Development workflow

- Development happens on the `V1.0` branch; `main` is protected and only receives changes
  through pull requests from `V1.0` whose CI checks pass.
- CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs on every push and pull request:
  lint, Go tests, `vendor/` consistency, builds with `DEPS=vendor` and `DEPS=online`, promu
  release archives, promtool rule checks and tests, dashboard generator sync, Docker image.
- Dependabot ([.github/dependabot.yml](.github/dependabot.yml)) proposes weekly updates of the
  Go modules (with `vendor/` regenerated), GitHub Actions and the Docker base image, on `V1.0`.
- Releases: set the version in `VERSION`, add its entry to [CHANGELOG.md](CHANGELOG.md)
  (checked by `promu check changelog`), merge into `main`, then push the matching tag
  (`git tag v$(cat VERSION) && git push origin v$(cat VERSION)`). The release workflow builds the archives
  (Linux amd64/arm64, Windows amd64, SHA-256 checksums) and publishes the GitHub release,
  with the changelog entry as release notes.
  `make release-tarballs` builds the same archives locally (needs promu).

## License

Apache License 2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).
