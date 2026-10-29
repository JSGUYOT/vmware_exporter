#!/usr/bin/env python3
"""Generate the four linked Grafana (>= 10) dashboards of vmware_exporter.

    vmware-overview.json   alarms, health, capacity, top lists       (uid vmware-overview)
    vmware-clusters.json   one or more clusters and their ESX hosts  (uid vmware-clusters)
    vmware-hosts.json      one or more ESX hosts and their VMs       (uid vmware-hosts)
    vmware-vms.json        one or more VMs                           (uid vmware-vms)

The dashboards share the tag "vmware_exporter": the header links (keeping time range and
variables) navigate between them. Data links drill down overview -> cluster -> ESX -> VM
(and back from a VM to its ESX host / cluster).

Regenerate (writes the four files next to this script):  python3 build_dashboard.py
"""
import json
import os

DS = {"type": "prometheus", "uid": "${datasource}"}
TAG = "vmware_exporter"
TITLE = "VMware vSphere / VCF - "

# key -> (uid, slug, title suffix, output file)
DASHBOARDS = {
    "overview": ("vmware-overview", "vmware-vsphere-vcf-overview", "Overview", "vmware-overview.json"),
    "clusters": ("vmware-clusters", "vmware-vsphere-vcf-clusters", "Clusters", "vmware-clusters.json"),
    "hosts": ("vmware-hosts", "vmware-vsphere-vcf-esx-hosts", "ESX hosts", "vmware-hosts.json"),
    "vms": ("vmware-vms", "vmware-vsphere-vcf-vms", "VMs", "vmware-vms.json"),
}


# --------------------------------------------------------------------------- selectors

def sel(*names, **fixed):
    """Label matchers: sel("vcenter", "host_name", state="dead") -> vcenter=~"$vcenter", host_name=~"$host_name", state="dead"."""
    parts = [f'{n}=~"${n}"' for n in names] + [f'{k}="{v}"' for k, v in fixed.items()]
    return ", ".join(parts)


SEL = sel("vcenter", "dc_name")                    # datacenter-level objects (datastores, alarms...)
SELC = sel("vcenter", "dc_name", "cluster_name")   # clusters, hosts, resource pools
SELH = sel("vcenter", "dc_name", "cluster_name", "host_name")
PI = 'perf_instance=""'           # aggregate instance of a perf counter
IN_CLUSTER = 'cluster_name!=""'  # hosts that belong to a cluster
VC = sel("vcenter")


def vm_in(info_sel):
    """VM series carry no host/cluster labels (stable across vMotion): filter them through vmware_vm_info."""
    return " and on(vcenter, vm_moid) vmware_vm_info{" + info_sel + "}"


def m(metric, s, extra=""):
    """metric{selector[, extra]}"""
    return metric + "{" + s + (", " + extra if extra else "") + "}"


def pct(num, den):
    return f"({num} / {den})"


# --------------------------------------------------------------------------- links

def drill(target, title, **vars_):
    """Data link to another dashboard: drill("clusters", "...", vcenter="${__data.fields.vcenter}", ...)."""
    uid, slug, _, _ = DASHBOARDS[target]
    q = "".join(f"&var-{k}={v}" for k, v in vars_.items())
    return {"title": title, "url": f"/d/{uid}/{slug}?var-datasource=${{datasource}}{q}&${{__url_time_range}}"}


# In tables (format=table): the clicked cell is ${__value.raw}, other columns ${__data.fields.<label>}.
T_CLUSTER = drill("clusters", "Open cluster ${__value.raw}", vcenter="${__data.fields.vcenter}", cluster_name="${__value.raw}")
T_HOST = drill("hosts", "Open ESX host ${__value.raw}", vcenter="${__data.fields.vcenter}", host_name="${__value.raw}")
T_VM = drill("vms", "Open VM ${__value.raw}", vcenter="${__data.fields.vcenter}", vm_name="${__value.raw}")
# In time series: the clicked series' labels.
S_CLUSTER = drill("clusters", "Open cluster ${__field.labels.cluster_name}",
                  vcenter="${__field.labels.vcenter}", cluster_name="${__field.labels.cluster_name}")
S_HOST = drill("hosts", "Open ESX host ${__field.labels.host_name}",
               vcenter="${__field.labels.vcenter}", host_name="${__field.labels.host_name}")
S_VM = drill("vms", "Open VM ${__field.labels.vm_name}", vcenter="${__field.labels.vcenter}", vm_name="${__field.labels.vm_name}")


# --------------------------------------------------------------------------- value mappings / thresholds

def vmap(**opts):
    """vmap(_0=("gray", "text"), _1=("green", "green")) -> value mappings (text + color)."""
    return [{"type": "value", "options": {k.lstrip("_"): {"text": t, "color": c, "index": i}
                                          for i, (k, (t, c)) in enumerate(opts.items())}}]


GRAY = "#8e8e8e"
M_STATUS = vmap(_0=("gray", GRAY), _1=("green", "green"), _2=("yellow", "orange"), _3=("red", "red"))
M_HOST_CONN = vmap(_0=("disconnected", "red"), _1=("connected", "green"), _2=("not responding", "red"))
M_VM_CONN = vmap(_0=("connected", "green"), _1=("disconnected", "red"), _2=("orphaned", "red"), _3=("inaccessible", "red"), _4=("invalid", "red"))
M_POWER = vmap(_0=("off", GRAY), _1=("on", "green"), _2=("suspended/standby", "orange"), _3=("unknown", GRAY))
M_YES_GOOD = vmap(_0=("no", "orange"), _1=("yes", "green"))   # 1 = expected (HA enabled, tools running...)
M_YES_BAD = vmap(_0=("no", "green"), _1=("yes", "orange"))    # 1 = needs attention (maintenance, reboot...)
M_YES = vmap(_0=("no", GRAY), _1=("yes", "blue"))

T_GREEN = [{"color": "green", "value": None}]
T_RED = [{"color": "green", "value": None}, {"color": "red", "value": 1}]
T_ORANGE = [{"color": "green", "value": None}, {"color": "orange", "value": 1}]
T_PCT = [{"color": "green", "value": None}, {"color": "orange", "value": 0.85}, {"color": "red", "value": 0.95}]
T_READY = [{"color": "green", "value": None}, {"color": "orange", "value": 0.05}, {"color": "red", "value": 0.10}]
T_LATENCY = [{"color": "green", "value": None}, {"color": "orange", "value": 0.02}, {"color": "red", "value": 0.05}]
T_EXPIRY = [{"color": "red", "value": None}, {"color": "orange", "value": 7 * 86400}, {"color": "green", "value": 30 * 86400}]
T_SNAP_AGE = [{"color": "green", "value": None}, {"color": "orange", "value": 3 * 86400}, {"color": "red", "value": 7 * 86400}]
T_FREE = [{"color": "red", "value": None}, {"color": "orange", "value": 0.10}, {"color": "green", "value": 0.15}]


# --------------------------------------------------------------------------- panels

def target(expr, ref, legend="", instant=False, fmt=None):
    t = {"refId": ref, "expr": expr, "datasource": DS}
    if legend:
        t["legendFormat"] = legend
    if instant:
        t["instant"], t["range"] = True, False
    if fmt:
        t["format"] = fmt
    return t


def stat(title, expr, thresholds=None, unit="none", color_mode="background", desc="", mappings=None, legend="", links=None, no_value=None):
    p = {
        "type": "stat", "title": title, "description": desc, "datasource": DS,
        "targets": [target(expr, "A", legend)],
        "fieldConfig": {"defaults": {"unit": unit, "mappings": mappings or [], "links": links or [],
                                     "thresholds": {"mode": "absolute", "steps": thresholds or T_GREEN}}, "overrides": []},
        "options": {"colorMode": color_mode, "graphMode": "none", "justifyMode": "auto", "textMode": "auto",
                    "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False}},
    }
    if no_value:
        p["fieldConfig"]["defaults"]["noValue"] = no_value
    return p


def ts(title, targets, unit="none", desc="", links=None, max_=None, min_=None, legend_calcs=("lastNotNull", "max")):
    """targets: [(expr, legendFormat), ...]"""
    d = {"unit": unit, "links": links or [], "custom": {"lineWidth": 1, "fillOpacity": 10, "showPoints": "never", "spanNulls": True}}
    if max_ is not None:
        d["max"] = max_
    if min_ is not None:
        d["min"] = min_
    if unit == "percentunit":
        d["custom"].update({"axisSoftMin": 0, "axisSoftMax": 1})
    return {
        "type": "timeseries", "title": title, "description": desc, "datasource": DS,
        "targets": [target(e, chr(65 + i), l) for i, (e, l) in enumerate(targets)],
        "fieldConfig": {"defaults": d, "overrides": []},
        "options": {"legend": {"displayMode": "table", "placement": "right", "calcs": list(legend_calcs), "showLegend": True},
                    "tooltip": {"mode": "multi", "sort": "desc"}},
    }


def bargauge(title, expr, legend, unit="percentunit", links=None):
    return {
        "type": "bargauge", "title": title, "datasource": DS,
        "targets": [target(expr, "A", legend, instant=True)],
        "fieldConfig": {"defaults": {"unit": unit, "min": 0, "max": 1, "links": links or [],
                                     "thresholds": {"mode": "absolute", "steps": T_PCT}}, "overrides": []},
        "options": {"displayMode": "gradient", "orientation": "horizontal", "showUnfilled": True,
                    "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False}},
    }


def col(header, expr, kind=None, unit=None, thresholds=None, by=(), hide_value=False):
    """One column of a multi-query table. kind: a mapping list, "pct" (gauge cell) or None.
    by: extra labels kept in addition to the table keys (info-style columns)."""
    return {"header": header, "expr": expr, "kind": kind, "unit": unit, "thresholds": thresholds, "by": list(by), "hide": hide_value}


def override(name, props):
    return {"matcher": {"id": "byName", "options": name}, "properties": [{"id": k, "value": v} for k, v in props.items()]}


def cell_props(c):
    p = {}
    if c["unit"]:
        p["unit"] = c["unit"]
    if isinstance(c["kind"], list):           # value mapping -> colored cell
        p["mappings"] = c["kind"]
        p["custom.cellOptions"] = {"type": "color-background", "mode": "basic"}
    elif c["kind"] == "pct":
        p.update({"unit": "percentunit", "min": 0, "max": 1, "thresholds": {"mode": "absolute", "steps": c["thresholds"] or T_PCT},
                  "custom.cellOptions": {"type": "gauge", "mode": "basic", "valueDisplayMode": "text"}})
    if c["thresholds"] and c["kind"] != "pct":
        p["thresholds"] = {"mode": "absolute", "steps": c["thresholds"]}
        p["custom.cellOptions"] = {"type": "color-text"}
    return p


def table(title, keys, cols, links=None, hide=(), sort=None, desc="", filterable=False, only=None, only_on=None):
    """Instant multi-query table: each column is `max by (keys + col.by) (expr)`, frames merged on the keys.
    only: optional PromQL; keep only the rows that also exist in it (`and on(only_on or keys) (only)`).
    links: {key label: data link} (the key columns keep their label name so ${__data.fields.<label>} resolves)."""
    targets, rename, index, exclude, overrides = [], {}, {}, {"Time": True}, []
    for k in hide:
        exclude[k] = True
    extras = [e for c in cols for e in c["by"] if e not in keys]
    order = list(dict.fromkeys(keys + extras))   # key labels, then info labels, then values
    index.update({k: i for i, k in enumerate(order)})
    nb = len(order)
    for i, c in enumerate(cols):
        ref = chr(65 + i)
        by = ", ".join(keys + c["by"])
        expr = f"max by ({by}) ({c['expr']})"
        if only:
            expr += f" and on({', '.join(only_on or keys)}) ({only})"
        targets.append(target(expr, ref, instant=True, fmt="table"))
        value = "Value" if len(cols) == 1 else f"Value #{ref}"
        if c["hide"]:
            exclude[value] = True
            continue
        rename[value] = c["header"]
        index[value] = nb + i
        props = cell_props(c)
        if props:
            overrides.append(override(c["header"], props))
    for k, link in (links or {}).items():
        overrides.append(override(k, {"links": [link]}))
    p = {
        "type": "table", "title": title, "description": desc, "datasource": DS, "targets": targets,
        "transformations": ([{"id": "merge", "options": {}}] if len(cols) > 1 else [])
        + [{"id": "organize", "options": {"excludeByName": exclude, "renameByName": rename, "indexByName": index}}],
        "fieldConfig": {"defaults": {"custom": {"filterable": filterable, "align": "auto"}}, "overrides": overrides},
        "options": {"showHeader": True, "cellHeight": "sm", "footer": {"show": False}},
    }
    if sort:
        p["options"]["sortBy"] = [{"displayName": sort[0], "desc": sort[1]}]
    return p


def raw_table(title, expr, columns, value_name="Value", kind=None, unit=None, thresholds=None, links=None, hide=(), sort=None, desc=""):
    """Single instant query displayed with all its labels (alarms, sensors, vSAN tests...)."""
    exclude = {"Time": True, "__name__": True, "job": True, "instance": True}
    for h in hide:
        exclude[h] = True
    c = col(value_name, "", kind, unit, thresholds)
    overrides = [override(value_name, cell_props(c))] if cell_props(c) else []
    for k, link in (links or {}).items():
        overrides.append(override(k, {"links": [link]}))
    p = {
        "type": "table", "title": title, "description": desc, "datasource": DS,
        "targets": [target(expr, "A", instant=True, fmt="table")],
        "transformations": [{"id": "organize", "options": {"excludeByName": exclude, "renameByName": {"Value": value_name},
                                                           "indexByName": {c: i for i, c in enumerate(columns)}}}],
        "fieldConfig": {"defaults": {"custom": {"filterable": True, "align": "auto"}}, "overrides": overrides},
        "options": {"showHeader": True, "cellHeight": "sm", "footer": {"show": False}},
    }
    if sort:
        p["options"]["sortBy"] = [{"displayName": sort[0], "desc": sort[1]}]
    return p


# --------------------------------------------------------------------------- dashboard / layout

def var(name, label, query, hide_all=False):
    v = {
        "name": name, "label": label, "type": "query", "datasource": DS, "refresh": 2, "includeAll": True, "multi": True,
        "allValue": ".*", "current": {"selected": True, "text": ["All"], "value": ["$__all"]},
        "query": {"query": query, "refId": name, "qryType": 1}, "definition": query, "sort": 1, "regex": "", "options": [],
    }
    return v


V_DS = {"name": "datasource", "label": "Data source", "type": "datasource", "query": "prometheus", "current": {}, "refresh": 1, "regex": ""}
V_VC = var("vcenter", "vCenter", "label_values(vmware_up, vcenter)")
V_DC = var("dc_name", "Datacenter", 'label_values(vmware_datacenter_overall_status{vcenter=~"$vcenter"}, dc_name)')
V_CLUSTER = var("cluster_name", "Cluster", "label_values(vmware_cluster_overall_status{" + SEL + "}, cluster_name)")
V_HOST = var("host_name", "ESX host", "label_values(vmware_host_info{" + SELC + "}, host_name)")
V_FOLDER = var("folder_name", "VM folder", "label_values(vmware_vm_info{" + SELH + "}, folder_name)")
V_VM = var("vm_name", "VM", "label_values(vmware_vm_info{" + SELH + ", folder_name=~\"$folder_name\"}, vm_name)")


class Dashboard:
    """Panels flow left to right (24 columns) and wrap; row() starts a new full-width section."""

    def __init__(self, key, variables, desc):
        self.key, self.variables, self.desc = key, variables, desc
        self.panels, self.x, self.y, self.line_h = [], 0, 0, 0

    def _newline(self):
        self.y += self.line_h
        self.x, self.line_h = 0, 0

    def row(self, title):
        self._newline()
        self.panels.append({"type": "row", "title": title, "collapsed": False, "panels": [], "gridPos": {"x": 0, "y": self.y, "w": 24, "h": 1}})
        self.y += 1

    def add(self, panel, w, h):
        if self.x + w > 24:
            self._newline()
        panel["gridPos"] = {"x": self.x, "y": self.y, "w": w, "h": h}
        self.panels.append(panel)
        self.x += w
        self.line_h = max(self.line_h, h)

    def json(self):
        uid, _, title, _ = DASHBOARDS[self.key]
        for i, p in enumerate(self.panels, 1):
            p["id"] = i
        return {
            "title": TITLE + title, "uid": uid, "description": self.desc,
            "tags": [TAG, "vmware", "vsphere", "vcf"],
            "links": [{"title": "VMware dashboards", "type": "dashboards", "tags": [TAG], "asDropdown": False,
                       "includeVars": True, "keepTime": True, "targetBlank": False, "icon": "external link", "tooltip": "", "url": ""}],
            "timezone": "browser", "schemaVersion": 39, "version": 1, "editable": True, "graphTooltip": 1,
            "refresh": "1m", "time": {"from": "now-6h", "to": "now"},
            "templating": {"list": [V_DS] + self.variables},
            "annotations": {"list": []},
            "panels": self.panels,
        }


# --------------------------------------------------------------------------- shared panel builders

def alarms_table(expr, title="Active vCenter alarms"):
    return raw_table(title, expr, ["vcenter", "dc_name", "status", "alarm", "entity_type", "entity", "acknowledged"],
                     value_name="Severity", kind=M_STATUS, hide=("alarm_moid", "entity_moid"), sort=("Severity", True),
                     desc="Active (triggered) vCenter alarms. Only present when an alarm is active.")


HOST_ALARM = 'vmware_alarm_triggered{' + VC + ', entity_type="HostSystem"}'
HOST_AS_ENTITY = 'label_replace(vmware_host_info{%s}, "entity", "$1", "host_name", "(.*)")'


def cluster_cols(s):
    return [
        col("Status", m("vmware_cluster_overall_status", s), M_STATUS),
        col("Hosts", m("vmware_cluster_hosts", s)),
        col("Effective hosts", m("vmware_cluster_hosts_effective", s)),
        col("VMs on", m("vmware_cluster_vms_powered_on", s)),
        col("VMs", m("vmware_cluster_vms", s)),
        col("CPU demand", pct(m("vmware_cluster_cpu_demand_mhz", s), m("vmware_cluster_cpu_effective_mhz", s)), "pct"),
        col("Memory demand", pct(m("vmware_cluster_memory_demand_bytes", s), m("vmware_cluster_memory_effective_bytes", s)), "pct"),
        col("HA", m("vmware_cluster_ha_enabled", s), M_YES_GOOD),
        col("DRS", m("vmware_cluster_drs_enabled", s), M_YES_GOOD),
        col("DRS fully auto", m("vmware_cluster_drs_fully_automated", s), M_YES),
        col("vSAN", m("vmware_cluster_vsan_enabled", s), M_YES),
        col("Config issues", m("vmware_cluster_config_issues", s), thresholds=T_ORANGE),
    ]


def host_cols(s):
    return [
        col("Info", m("vmware_host_info", s), by=("version", "build", "model"), hide_value=True),
        col("Status", m("vmware_host_overall_status", s), M_STATUS),
        col("Connection", m("vmware_host_connection_status", s), M_HOST_CONN),
        col("Maintenance", m("vmware_host_in_maintenance", s), M_YES_BAD),
        col("CPU", pct(m("vmware_host_cpu_usage_mhz", s), m("vmware_host_cpu_total_mhz", s)), "pct"),
        col("Memory", pct(m("vmware_host_memory_usage_bytes", s), m("vmware_host_memory_total_bytes", s)), "pct"),
        col("VMs", m("vmware_host_vms", s)),
        col("Uptime", m("vmware_host_uptime_seconds", s), unit="s"),
        col("Reboot required", m("vmware_host_reboot_required", s), M_YES_BAD),
        col("Config issues", m("vmware_host_config_issues", s), thresholds=T_ORANGE),
    ]


def vm_cols(vs, info):
    """VM table columns; vs = VM series selector, info = vmware_vm_info selector (host/cluster filter)."""
    f = vm_in(info)
    return [
        col("Placement", m("vmware_vm_info", info), by=("host_name", "cluster_name", "guest_os", "ip_address"), hide_value=True),
        col("Power", m("vmware_vm_power_status", vs) + f, M_POWER),
        col("Status", m("vmware_vm_overall_status", vs) + f, M_STATUS),
        col("vCPUs", m("vmware_vm_vcpus", vs) + f),
        col("Memory", m("vmware_vm_memory_size_bytes", vs) + f, unit="bytes"),
        col("CPU used", m("vmware_vm_cpu_usage_mhz", vs) + f, unit="mhz"),
        col("CPU ready / vCPU", "(" + ready_per_vcpu(vs) + ")" + f, "pct", thresholds=T_READY),
        col("Host memory consumed", m("vmware_vm_memory_host_consumed_bytes", vs) + f, unit="bytes"),
        col("Tools running", m("vmware_vm_tools_running", vs) + f, M_YES_GOOD),
        col("Snapshots", m("vmware_vm_snapshots_existing", vs) + f, thresholds=T_ORANGE),
        col("Uptime", m("vmware_vm_uptime_seconds", vs) + f, unit="s"),
    ]


def ready_per_vcpu(vs):
    """CPU ready (summation over the 20 s real-time sample) as a fraction of time, per vCPU."""
    return "(" + m("vmware_vm_perf_cpu_ready_summation_seconds", vs, 'perf_instance=""') + " / 20) / ignoring(perf_instance) " + m("vmware_vm_vcpus", vs)


# =========================================================================== 1. Overview

def overview():
    d = Dashboard("overview", [V_VC, V_DC, V_CLUSTER],
                  "Global health of the vCenters: alarms, capacity, top lists. Click a cluster / host / VM to drill down.")
    vf = vm_in(SELC)

    d.row("Overview")
    d.add(stat("vCenters reachable", f"sum(vmware_up{{{VC}}}) / count(vmware_up{{{VC}}})", unit="percentunit",
               thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 1}]), 4, 4)
    d.add(stat("Red alarms", f'sum(vmware_alarms_active{{{VC}, status="red"}}) or vector(0)', T_RED), 4, 4)
    d.add(stat("Yellow alarms", f'sum(vmware_alarms_active{{{VC}, status="yellow"}}) or vector(0)', T_ORANGE), 4, 4)
    d.add(stat("Hosts connected", f'count(vmware_host_connection_status{{{SELC}}} == 1) or vector(0)', color_mode="value"), 4, 4)
    d.add(stat("Hosts not connected", f'count(vmware_host_connection_status{{{SELC}}} != 1) or vector(0)', T_RED), 4, 4)
    d.add(stat("VMs powered on", f"count((vmware_vm_power_status{{{SEL}}} == 1){vf}) or vector(0)", color_mode="value"), 4, 4)
    d.add(stat("Datastores > 85%", f"count((1 - vmware_datastore_free_bytes{{{SEL}}} / vmware_datastore_capacity_bytes{{{SEL}}}) > 0.85) or vector(0)", T_ORANGE), 4, 4)
    d.add(stat("Snapshots > 3 days", f"count((vmware_vm_snapshot_oldest_age_seconds{{{SEL}}} > 259200){vf}) or vector(0)", T_ORANGE), 4, 4)
    d.add(stat("VMs without Tools", f"count((vmware_vm_tools_running{{{SEL}}} == 0) and on(vcenter, vm_moid) (vmware_vm_power_status == 1){vf}) or vector(0)", T_ORANGE), 4, 4)
    d.add(stat("Dead storage paths", f'sum(vmware_host_storage_paths{{{SELC}, state="dead"}}) or vector(0)', T_RED), 4, 4)
    d.add(stat("vSAN health (worst)", f"max(vmware_vsan_cluster_health{{{SELC}}}) or vector(0)", mappings=M_STATUS,
               thresholds=[{"color": "text", "value": None}, {"color": "green", "value": 1}, {"color": "orange", "value": 2}, {"color": "red", "value": 3}],
               desc="0 = no vSAN / unknown, 1 = green, 2 = yellow, 3 = red"), 4, 4)
    d.add(stat("Failing collectors", f"count(vmware_collector_success{{{VC}}} == 0) or vector(0)", T_ORANGE), 4, 4)

    d.row("vCenters")
    d.add(table("vCenters", ["vcenter"], [
        col("Info", m("vmware_vcenter_info", VC), by=("version", "build"), hide_value=True),
        col("Up", m("vmware_up", VC), M_YES_GOOD),
        col("Sessions", m("vmware_vcenter_sessions", VC)),
        col("Certificate expires in", m("vmware_vcenter_certificate_expiry_timestamp_seconds", VC) + " - time()", unit="s", thresholds=T_EXPIRY),
        col("Hosts", f'count by (vcenter) (vmware_host_info{{{VC}}})'),
        col("VMs monitored", m("vmware_vms_monitored", VC)),
        col("VMs powered on", m("vmware_vms_powered_on", VC)),
        col("Collection duration", m("vmware_collection_duration_seconds", VC), unit="s"),
    ]), 24, 5)

    d.row("Active vCenter alarms")
    d.add(alarms_table(m("vmware_alarm_triggered", SEL)), 24, 8)

    d.row("Clusters")
    d.add(table("Clusters (click a cluster to open it)", ["vcenter", "dc_name", "cluster_name"], cluster_cols(SELC),
                links={"cluster_name": T_CLUSTER}, sort=("CPU demand", True)), 24, 7)
    d.add(ts("CPU: demand / effective capacity", [(pct(m("vmware_cluster_cpu_demand_mhz", SELC), m("vmware_cluster_cpu_effective_mhz", SELC)),
                                                   "{{vcenter}} / {{cluster_name}}")], "percentunit", links=[S_CLUSTER]), 12, 8)
    d.add(ts("Memory: demand / effective capacity", [(pct(m("vmware_cluster_memory_demand_bytes", SELC), m("vmware_cluster_memory_effective_bytes", SELC)),
                                                      "{{vcenter}} / {{cluster_name}}")], "percentunit", links=[S_CLUSTER]), 12, 8)

    d.row("ESX hosts")
    d.add(ts("Top 10 ESX CPU usage", [(f"topk(10, {pct(m('vmware_host_cpu_usage_mhz', SELC), m('vmware_host_cpu_total_mhz', SELC))})",
                                      "{{host_name}}")], "percentunit", links=[S_HOST]), 12, 8)
    d.add(ts("Top 10 ESX memory usage", [(f"topk(10, {pct(m('vmware_host_memory_usage_bytes', SELC), m('vmware_host_memory_total_bytes', SELC))})",
                                         "{{host_name}}")], "percentunit", links=[S_HOST]), 12, 8)
    bad = (f"(vmware_host_overall_status{{{SELC}}} != 1) or (vmware_host_connection_status{{{SELC}}} != 1) "
           f"or (vmware_host_in_maintenance{{{SELC}}} == 1) or (vmware_host_reboot_required{{{SELC}}} == 1)")
    d.add(table("ESX hosts needing attention (not green, not connected, maintenance, reboot required)",
                ["vcenter", "dc_name", "cluster_name", "host_name"], host_cols(SELC)[1:4] + [col("Reboot required", m("vmware_host_reboot_required", SELC), M_YES_BAD)],
                links={"host_name": T_HOST, "cluster_name": T_CLUSTER}, only=bad, desc="Empty = every ESX host is fine."), 24, 6)

    d.row("Datastores")
    d.add(bargauge("Datastore usage", f"1 - vmware_datastore_free_bytes{{{SEL}}} / vmware_datastore_capacity_bytes{{{SEL}}}", "{{vcenter}} / {{ds_name}}"), 12, 10)
    d.add(table("Datastores", ["vcenter", "dc_name", "ds_name"], [
        col("Info", m("vmware_datastore_info", SEL), by=("type", "shared", "datastore_cluster"), hide_value=True),
        col("Status", m("vmware_datastore_overall_status", SEL), M_STATUS),
        col("Accessible", m("vmware_datastore_is_accessible", SEL), M_YES_GOOD),
        col("Capacity", m("vmware_datastore_capacity_bytes", SEL), unit="bytes"),
        col("Free", m("vmware_datastore_free_bytes", SEL), unit="bytes"),
        col("Used", f"1 - vmware_datastore_free_bytes{{{SEL}}} / vmware_datastore_capacity_bytes{{{SEL}}}", "pct"),
        col("Provisioned", pct(m("vmware_datastore_provisioned_bytes", SEL), m("vmware_datastore_capacity_bytes", SEL)), unit="percentunit"),
        col("Hosts", m("vmware_datastore_hosts_mounted", SEL)),
        col("VMs", m("vmware_datastore_vms_hosted", SEL)),
    ], sort=("Used", True), filterable=True), 12, 10)

    d.row("Virtual machines")
    d.add(ts("Top 10 CPU ready (% per vCPU)", [(f"topk(10, ({ready_per_vcpu(SEL)}){vf})", "{{vm_name}}")], "percentunit", links=[S_VM]), 12, 8)
    d.add(ts("Top 10 disk latency", [(f"topk(10, vmware_vm_perf_disk_max_total_latency_latest_seconds{{{SEL}}}{vf})", "{{vm_name}}")], "s", links=[S_VM]), 12, 8)
    d.add(ts("Top 10 CPU usage (MHz)", [(f"topk(10, vmware_vm_cpu_usage_mhz{{{SEL}}}{vf})", "{{vm_name}}")], "mhz", links=[S_VM]), 12, 8)
    d.add(ts("Swapped / ballooned memory", [(f"sum by (vcenter, vm_name) (vmware_vm_memory_swapped_bytes{{{SEL}}}{vf}) > 0", "swap {{vm_name}}"),
                                            (f"sum by (vcenter, vm_name) (vmware_vm_memory_ballooned_bytes{{{SEL}}}{vf}) > 0", "balloon {{vm_name}}")],
             "bytes", links=[S_VM], desc="Only VMs with swapped or ballooned memory (empty = none)."), 12, 8)
    d.add(raw_table("Snapshots (oldest per VM)", f"vmware_vm_snapshot_oldest_age_seconds{{{SEL}}}{vf}", ["vcenter", "dc_name", "vm_name"],
                    value_name="Oldest age", unit="s", thresholds=T_SNAP_AGE, hide=("vm_moid", "folder_name"), sort=("Oldest age", True),
                    links={"vm_name": T_VM}), 12, 8)
    d.add(raw_table("Guest disks < 15% free", f"((vmware_vm_guest_disk_free_bytes{{{SEL}}} / vmware_vm_guest_disk_capacity_bytes{{{SEL}}}) < 0.15){vf}",
                    ["vcenter", "vm_name", "disk_path"], value_name="Free", kind="pct", thresholds=T_FREE, hide=("vm_moid", "folder_name", "dc_name"),
                    links={"vm_name": T_VM}), 12, 8)

    d.row("vSAN")
    d.add(raw_table("Non-green vSAN health tests", f"vmware_vsan_health_test{{{SELC}}} > 1", ["vcenter", "cluster_name", "group", "test"],
                    value_name="Severity", kind=M_STATUS, links={"cluster_name": T_CLUSTER}), 24, 8)

    d.row("Exporter")
    d.add(ts("Collection duration", [(f"vmware_collection_duration_seconds{{{VC}}}", "{{vcenter}}")], "s"), 12, 8)
    d.add(raw_table("Collectors", f"vmware_collector_success{{{VC}}}", ["vcenter", "collector"], value_name="OK", kind=M_YES_GOOD), 12, 8)
    return d


# =========================================================================== 2. Clusters

def clusters():
    d = Dashboard("clusters", [V_VC, V_DC, V_CLUSTER],
                  "Selected cluster(s): status, capacity, ESX hosts (click a host to open it), resource pools, vSAN, alarms.")
    S = SELC
    vf = vm_in(SELC)
    hk = "vcenter, cluster_name"

    d.row("Status")
    d.add(table("Cluster status", ["vcenter", "dc_name", "cluster_name"], cluster_cols(S) + [
        col("Datastores", m("vmware_cluster_datastores", S)),
        col("DRS recommendations", m("vmware_cluster_drs_recommendations", S), thresholds=T_ORANGE),
        col("vMotions (range)", f"increase(vmware_cluster_vmotions_total{{{S}}}[$__range])"),
        col("Active alarms", f"count by (vcenter, dc_name, cluster_name) (label_replace(vmware_alarm_triggered{{{VC}, entity_type=\"ClusterComputeResource\"}}, \"cluster_name\", \"$1\", \"entity\", \"(.*)\"))",
            thresholds=T_RED),
    ]), 24, 6)

    d.row("Capacity")
    d.add(ts("CPU capacity (MHz)", [
        (m("vmware_cluster_cpu_total_mhz", S), "{{cluster_name}} total"),
        (m("vmware_cluster_cpu_effective_mhz", S), "{{cluster_name}} effective"),
        (m("vmware_cluster_cpu_demand_mhz", S), "{{cluster_name}} demand"),
        (m("vmware_cluster_cpu_entitled_mhz", S), "{{cluster_name}} entitled"),
        (m("vmware_cluster_cpu_reserved_mhz", S), "{{cluster_name}} reserved"),
    ], "mhz"), 12, 8)
    d.add(ts("Memory capacity", [
        (m("vmware_cluster_memory_total_bytes", S), "{{cluster_name}} total"),
        (m("vmware_cluster_memory_effective_bytes", S), "{{cluster_name}} effective"),
        (m("vmware_cluster_memory_demand_bytes", S), "{{cluster_name}} demand"),
        (m("vmware_cluster_memory_entitled_bytes", S), "{{cluster_name}} entitled"),
        (m("vmware_cluster_memory_reserved_bytes", S), "{{cluster_name}} reserved"),
    ], "bytes"), 12, 8)
    n1 = lambda used, total: (f"sum by ({hk}) ({m(used, S, IN_CLUSTER)}) / "
                              f"(sum by ({hk}) ({m(total, S, IN_CLUSTER)}) - max by ({hk}) ({m(total, S, IN_CLUSTER)}))")
    d.add(ts("Usage if the largest host fails (N-1)", [
        (n1("vmware_host_cpu_usage_mhz", "vmware_host_cpu_total_mhz"), "{{cluster_name}} CPU"),
        (n1("vmware_host_memory_usage_bytes", "vmware_host_memory_total_bytes"), "{{cluster_name}} memory"),
    ], "percentunit", desc="Current ESX usage divided by the capacity left without the largest host. > 100% = the cluster cannot absorb a host failure."), 12, 8)
    d.add(ts("HA failover resources available / DRS score", [
        (m("vmware_cluster_ha_failover_cpu_resources_percent", S), "{{cluster_name}} HA CPU available %"),
        (m("vmware_cluster_ha_failover_memory_resources_percent", S), "{{cluster_name}} HA memory available %"),
        (m("vmware_cluster_drs_score", S), "{{cluster_name}} DRS score"),
    ], "none", min_=0, max_=100, desc="HA admission control (percentage policy) and DRS score (0-100). Only on real vCenters."), 12, 8)

    d.row("ESX hosts")
    d.add(table("ESX hosts (click a host to open it)", ["vcenter", "cluster_name", "host_name"], host_cols(S),
                links={"host_name": T_HOST}, sort=("CPU", True), filterable=True), 24, 8)
    d.add(ts("CPU usage per host", [(pct(m("vmware_host_cpu_usage_mhz", S), m("vmware_host_cpu_total_mhz", S)), "{{host_name}}")],
             "percentunit", links=[S_HOST], max_=1), 12, 8)
    d.add(ts("Memory usage per host", [(pct(m("vmware_host_memory_usage_bytes", S), m("vmware_host_memory_total_bytes", S)), "{{host_name}}")],
             "percentunit", links=[S_HOST], max_=1), 12, 8)
    d.add(ts("Network per host (rx + tx)", [(f"{m('vmware_host_perf_net_received_average_bytes_per_second', S, PI)} + "
                                             f"{m('vmware_host_perf_net_transmitted_average_bytes_per_second', S, PI)}", "{{host_name}}")],
             "Bps", links=[S_HOST]), 12, 8)
    d.add(ts("Max disk latency per host", [(f"max by (vcenter, host_name) ({m('vmware_host_perf_disk_max_total_latency_latest_seconds', S)})", "{{host_name}}")],
             "s", links=[S_HOST]), 12, 8)

    d.row("Virtual machines")
    d.add(ts("Top 10 VMs by CPU ready (% per vCPU)", [(f"topk(10, ({ready_per_vcpu(SEL)}){vf})", "{{vm_name}}")], "percentunit", links=[S_VM]), 12, 8)
    d.add(ts("Top 10 VMs by CPU usage (MHz)", [(f"topk(10, vmware_vm_cpu_usage_mhz{{{SEL}}}{vf})", "{{vm_name}}")], "mhz", links=[S_VM]), 12, 8)

    d.row("Resource pools")
    d.add(table("Resource pools", ["vcenter", "cluster_name", "resource_pool"], [
        col("Status", m("vmware_resourcepool_overall_status", S), M_STATUS),
        col("VMs", m("vmware_resourcepool_vms", S)),
        col("CPU reservation", m("vmware_resourcepool_cpu_reservation_mhz", S), unit="mhz"),
        col("CPU limit (-1 = none)", m("vmware_resourcepool_cpu_limit_mhz", S), unit="mhz"),
        col("Memory reservation", m("vmware_resourcepool_memory_reservation_bytes", S), unit="bytes"),
        col("Memory limit (-1 = none)", m("vmware_resourcepool_memory_limit_bytes", S), unit="bytes"),
    ], filterable=True), 24, 7)

    d.row("vSAN")
    d.add(stat("vSAN health", f"vmware_vsan_cluster_health{{{S}}}", mappings=M_STATUS, legend="{{cluster_name}}",
               thresholds=[{"color": "text", "value": None}, {"color": "green", "value": 1}, {"color": "orange", "value": 2}, {"color": "red", "value": 3}],
               desc="Only on vSAN clusters.", no_value="no vSAN"), 6, 6)
    d.add(raw_table("vSAN health tests (worst first)", f"vmware_vsan_health_test{{{S}}}", ["vcenter", "cluster_name", "group", "test"],
                    value_name="Severity", kind=M_STATUS, sort=("Severity", True)), 18, 6)

    d.row("Alarms on the cluster and its hosts")
    d.add(alarms_table(f'(label_replace(vmware_alarm_triggered{{{VC}, entity_type="ClusterComputeResource"}}, "cluster_name", "$1", "entity", "(.*)") '
                       f"and on(vcenter, cluster_name) vmware_cluster_overall_status{{{S}}}) "
                       f"or ({HOST_ALARM} and on(vcenter, entity) {HOST_AS_ENTITY % S})", "Active alarms"), 24, 8)
    return d


# =========================================================================== 3. ESX hosts

def hosts():
    d = Dashboard("hosts", [V_VC, V_DC, V_CLUSTER, V_HOST],
                  "Selected ESX host(s): status, CPU / memory, storage, network, hardware, VMs (click a VM to open it).")
    S = SELH
    vs = sel("vcenter", "dc_name")
    info = SELH

    def perf(metric, legend, extra='perf_instance=""'):
        return (m(metric, S, extra), legend)

    d.row("Status")
    d.add(table("ESX host status", ["vcenter", "cluster_name", "host_name"], host_cols(S) + [
        col("Power", m("vmware_host_power_status", S), M_POWER),
        col("Quarantine", m("vmware_host_quarantine_mode", S), M_YES_BAD),
        col("Certificate expires in", m("vmware_host_certificate_expiry_timestamp_seconds", S) + " - time()", unit="s", thresholds=T_EXPIRY),
        col("Active alarms", f"count by (vcenter, cluster_name, host_name) ({HOST_ALARM} * on(vcenter, entity) group_left(cluster_name, host_name) {HOST_AS_ENTITY % S})",
            thresholds=T_RED),
    ], links={"host_name": T_HOST, "cluster_name": T_CLUSTER}), 24, 6)
    d.add(table("Hardware", ["vcenter", "host_name"], [
        col("Info", m("vmware_host_info", S), by=("vendor", "model", "cpu_model", "bios_version", "product"), hide_value=True),
        col("Cores", m("vmware_host_cpu_cores", S)),
        col("Threads", m("vmware_host_cpu_threads", S)),
        col("CPU total", m("vmware_host_cpu_total_mhz", S), unit="mhz"),
        col("Memory", m("vmware_host_memory_total_bytes", S), unit="bytes"),
        col("Datastores", m("vmware_host_datastores", S)),
        col("LUNs", m("vmware_host_storage_luns", S)),
    ]), 24, 5)

    d.row("CPU / memory")
    d.add(ts("CPU usage", [(pct(m("vmware_host_cpu_usage_mhz", S), m("vmware_host_cpu_total_mhz", S)), "{{host_name}} usage"),
                           (f"{m('vmware_host_perf_cpu_utilization_average_percent', S, PI)} / 100", "{{host_name}} utilization (perf)"),
                           (f"{m('vmware_host_perf_cpu_core_utilization_average_percent', S, PI)} / 100", "{{host_name}} core utilization (perf)")],
             "percentunit", min_=0, max_=1), 12, 8)
    d.add(ts("Memory usage", [(pct(m("vmware_host_memory_usage_bytes", S), m("vmware_host_memory_total_bytes", S)), "{{host_name}} usage"),
                              (f"{m('vmware_host_perf_mem_usage_average_percent', S, PI)} / 100", "{{host_name}} usage (perf)")],
             "percentunit", min_=0, max_=1), 12, 8)
    d.add(ts("Memory (perf)", [perf("vmware_host_perf_mem_consumed_average_bytes", "{{host_name}} consumed"),
                               perf("vmware_host_perf_mem_active_average_bytes", "{{host_name}} active"),
                               perf("vmware_host_perf_mem_vmmemctl_average_bytes", "{{host_name}} ballooned")], "bytes"), 12, 8)
    d.add(ts("Hypervisor swap rate", [perf("vmware_host_perf_mem_swapin_rate_average_bytes_per_second", "{{host_name}} swap in"),
                                      perf("vmware_host_perf_mem_swapout_rate_average_bytes_per_second", "{{host_name}} swap out")], "Bps",
             desc="Should stay at 0: host memory overcommitment."), 12, 8)

    d.row("Storage")
    d.add(ts("Disk throughput", [perf("vmware_host_perf_disk_read_average_bytes_per_second", "{{host_name}} read"),
                                 perf("vmware_host_perf_disk_write_average_bytes_per_second", "{{host_name}} write")], "Bps"), 12, 8)
    d.add(ts("Max disk latency", [(f"max by (vcenter, host_name) ({m('vmware_host_perf_disk_max_total_latency_latest_seconds', S)})", "{{host_name}}")], "s"), 12, 8)
    lat = f'max by (vcenter, host_name, perf_instance) ({{__name__=~"vmware_host_perf_datastore_total_(read|write)_latency_average_seconds", {S}, perf_instance!=""}})'
    dsmap = ('max by (vcenter, perf_instance, ds_name) (label_replace(vmware_datastore_info{' + VC + ', url=~".*/volumes/.+"}, '
             '"perf_instance", "$1", "url", ".*/volumes/([^/]+)/?"))')
    d.add(ts("Datastore latency seen by the host (max of read / write)", [
        (f"{lat} * on(vcenter, perf_instance) group_left(ds_name) {dsmap}", "{{host_name}} / {{ds_name}}"),
        (f"{lat} unless on(vcenter, perf_instance) {dsmap}", "{{host_name}} / {{perf_instance}}"),
    ], "s", desc="Per datastore (datastore.totalRead/WriteLatency counters, instance = datastore UUID resolved to its name)."), 12, 8)
    d.add(ts("Storage paths / LUNs", [(f"sum by (vcenter, host_name, state) ({m('vmware_host_storage_paths', S)})", "{{host_name}} paths {{state}}"),
                                      (m("vmware_host_storage_luns_degraded", S), "{{host_name}} LUNs degraded"),
                                      (m("vmware_host_storage_luns_no_path", S), "{{host_name}} LUNs without path")], "none"), 12, 8)
    d.add(table("Datastores of the host's datacenter", ["vcenter", "dc_name", "ds_name"], [
        col("Info", m("vmware_datastore_info", SEL), by=("type", "shared"), hide_value=True),
        col("Status", m("vmware_datastore_overall_status", SEL), M_STATUS),
        col("Accessible", m("vmware_datastore_is_accessible", SEL), M_YES_GOOD),
        col("Capacity", m("vmware_datastore_capacity_bytes", SEL), unit="bytes"),
        col("Free", m("vmware_datastore_free_bytes", SEL), unit="bytes"),
        col("Used", f"1 - vmware_datastore_free_bytes{{{SEL}}} / vmware_datastore_capacity_bytes{{{SEL}}}", "pct"),
        col("Hosts mounting", m("vmware_datastore_hosts_mounted", SEL)),
        col("Hosts without access", m("vmware_datastore_hosts_inaccessible", SEL), thresholds=T_RED),
    ], sort=("Used", True), filterable=True,
        only=f"vmware_host_info{{{S}}}", only_on=["vcenter", "dc_name"],
        desc="Datastores of the datacenter(s) containing the selected host(s) (the exporter does not expose the host/datastore mount map)."), 24, 8)

    d.row("Network")
    d.add(ts("Network throughput", [perf("vmware_host_perf_net_received_average_bytes_per_second", "{{host_name}} rx"),
                                    perf("vmware_host_perf_net_transmitted_average_bytes_per_second", "{{host_name}} tx")], "Bps"), 12, 8)
    d.add(ts("Dropped packets / errors (per 20 s sample)", [
        perf("vmware_host_perf_net_dropped_rx_summation", "{{host_name}} dropped rx"),
        perf("vmware_host_perf_net_dropped_tx_summation", "{{host_name}} dropped tx"),
        perf("vmware_host_perf_net_errors_rx_summation", "{{host_name}} errors rx"),
        perf("vmware_host_perf_net_errors_tx_summation", "{{host_name}} errors tx")], "none"), 12, 8)
    d.add(table("Physical NICs", ["vcenter", "host_name", "nic"], [
        col("Driver", m("vmware_host_pnic_link_up", S), by=("driver", "used"), hide_value=True),
        col("Link", m("vmware_host_pnic_link_up", S), vmap(_0=("down", "red"), _1=("up", "green"))),
        col("Speed", m("vmware_host_pnic_speed_mbps", S) + " * 1000000", unit="bps"),
    ], filterable=True), 24, 7)

    d.row("Hardware health / services")
    d.add(ts("Power consumption", [perf("vmware_host_perf_power_power_average_watts", "{{host_name}}")], "watt"), 8, 8)
    d.add(raw_table("Hardware status", f"vmware_host_hardware_status{{{S}}}", ["host_name", "component", "name"],
                    value_name="Status", kind=M_STATUS, hide=("vcenter", "dc_name", "cluster_name"), sort=("Status", True)), 16, 8)
    d.add(table("Sensors (worst first)", ["host_name", "type", "sensor"], [
        col("Health", m("vmware_host_sensor_health", S), M_STATUS),
        col("Unit", m("vmware_host_sensor_value", S), by=("unit",), hide_value=True),
        col("Value", m("vmware_host_sensor_value", S)),
    ], sort=("Health", True), filterable=True), 16, 8)
    d.add(raw_table("Services", f"vmware_host_service_running{{{S}}}", ["host_name", "service", "policy"],
                    value_name="Running", kind=vmap(_0=("stopped", GRAY), _1=("running", "green")), hide=("vcenter", "dc_name", "cluster_name")), 8, 8)

    d.row("Virtual machines on the host")
    d.add(table("VMs (click a VM to open it)", ["vcenter", "vm_name", "vm_moid"], vm_cols(vs, info),
                links={"vm_name": T_VM, "host_name": T_HOST, "cluster_name": T_CLUSTER}, hide=("vm_moid",), sort=("CPU used", True), filterable=True), 24, 10)
    vf = vm_in(info)
    d.add(ts("Top 10 VMs by CPU ready (% per vCPU)", [(f"topk(10, ({ready_per_vcpu(vs)}){vf})", "{{vm_name}}")], "percentunit", links=[S_VM]), 12, 8)
    d.add(ts("Top 10 VMs by CPU usage (MHz)", [(f"topk(10, vmware_vm_cpu_usage_mhz{{{vs}}}{vf})", "{{vm_name}}")], "mhz", links=[S_VM]), 12, 8)

    d.row("Alarms")
    d.add(alarms_table(f"{HOST_ALARM} and on(vcenter, entity) {HOST_AS_ENTITY % S}", "Active alarms on the host"), 24, 7)
    return d


# =========================================================================== 4. VMs

def vms():
    d = Dashboard("vms", [V_VC, V_DC, V_CLUSTER, V_HOST, V_FOLDER, V_VM],
                  "Selected VM(s): placement (links back to the ESX host / cluster), CPU, memory, disks, network, snapshots, tools.")
    vs = sel("vcenter", "dc_name", "folder_name", "vm_name")
    info = SELH + ', folder_name=~"$folder_name", vm_name=~"$vm_name"'
    vf = vm_in(info)

    def q(metric, extra=""):
        # parenthesized: `and` has the lowest precedence, `q(..) > 0` must compare the VM series, not vmware_vm_info
        return "(" + m(metric, vs, extra) + vf + ")"

    def top(expr):
        return f"topk(25, {expr})"

    def perf(metric, legend):
        return (top(q(metric, 'perf_instance=""')), legend)

    d.row("Summary")
    d.add(stat("VMs", f"count({q('vmware_vm_power_status')}) or vector(0)", color_mode="value"), 4, 4)
    d.add(stat("Powered on", f"count({q('vmware_vm_power_status')} == 1) or vector(0)", color_mode="value"), 4, 4)
    d.add(stat("Tools not running (powered on)", f"count(({q('vmware_vm_tools_running')} == 0) and on(vcenter, vm_moid) (vmware_vm_power_status == 1)) or vector(0)", T_ORANGE), 4, 4)
    d.add(stat("With snapshots", f"count({q('vmware_vm_snapshots_existing')} > 0) or vector(0)", T_ORANGE), 4, 4)
    d.add(stat("Consolidation needed", f"count({q('vmware_vm_consolidation_needed')} == 1) or vector(0)", T_RED), 4, 4)
    d.add(stat("Question pending", f"count({q('vmware_vm_question_pending')} == 1) or vector(0)", T_RED), 4, 4)
    d.add(table("VMs (placement: click the ESX host / cluster to open it)", ["vcenter", "vm_name", "vm_moid"], vm_cols(vs, info) + [
        col("Connection", m("vmware_vm_connection_state", vs) + vf, M_VM_CONN),
        col("Heartbeat", m("vmware_vm_guest_heartbeat_status", vs) + vf, M_STATUS),
        col("Tools version", m("vmware_vm_tools_version_status", vs) + vf, by=("status",), hide_value=True),
        col("Consolidation needed", m("vmware_vm_consolidation_needed", vs) + vf, M_YES_BAD),
        col("Question pending", m("vmware_vm_question_pending", vs) + vf, M_YES_BAD),
    ], links={"vm_name": T_VM, "host_name": T_HOST, "cluster_name": T_CLUSTER}, hide=("vm_moid",), filterable=True), 24, 9)

    d.row("CPU")
    d.add(ts("CPU usage (MHz)", [(top(q("vmware_vm_cpu_usage_mhz")), "{{vm_name}} used"),
                                 (top(q("vmware_vm_cpu_demand_mhz")), "{{vm_name}} demand"),
                                 (top(q("vmware_vm_cpu_max_mhz")), "{{vm_name}} max")], "mhz",
             desc="Top 25 VMs when several are selected."), 12, 8)
    d.add(ts("CPU ready / co-stop / latency", [
        (top(f"({ready_per_vcpu(vs)}){vf}"), "{{vm_name}} ready per vCPU"),
        (top(f"{q('vmware_vm_perf_cpu_readiness_average_percent', PI)} / 100"), "{{vm_name}} readiness"),
        (top(f"{q('vmware_vm_perf_cpu_latency_average_percent', PI)} / 100"), "{{vm_name}} latency"),
        (top(f"({m('vmware_vm_perf_cpu_costop_summation_seconds', vs, PI)} / 20) / ignoring(perf_instance) {m('vmware_vm_vcpus', vs)}{vf}"),
         "{{vm_name}} co-stop per vCPU"),
    ], "percentunit", desc="Ready > 5% per vCPU: CPU contention on the ESX host. Co-stop: too many vCPUs."), 12, 8)

    d.row("Memory")
    d.add(ts("Memory", [(top(q("vmware_vm_memory_size_bytes")), "{{vm_name}} configured"),
                        (top(q("vmware_vm_memory_host_consumed_bytes")), "{{vm_name}} host consumed"),
                        (top(q("vmware_vm_memory_guest_active_bytes")), "{{vm_name}} guest active")], "bytes"), 12, 8)
    d.add(ts("Memory reclamation (ballooned / swapped / compressed)", [
        (top(q("vmware_vm_memory_ballooned_bytes")), "{{vm_name}} ballooned"),
        (top(q("vmware_vm_memory_swapped_bytes")), "{{vm_name}} swapped"),
        (top(q("vmware_vm_memory_compressed_bytes")), "{{vm_name}} compressed")], "bytes", desc="Should stay at 0."), 12, 8)

    d.row("Disks")
    d.add(ts("Disk throughput", [perf("vmware_vm_perf_disk_read_average_bytes_per_second", "{{vm_name}} read"),
                                 perf("vmware_vm_perf_disk_write_average_bytes_per_second", "{{vm_name}} write")], "Bps"), 12, 8)
    d.add(ts("Max disk latency", [(top(q("vmware_vm_perf_disk_max_total_latency_latest_seconds")), "{{vm_name}}")], "s"), 12, 8)
    d.add(table("Storage", ["vcenter", "vm_name", "vm_moid"], [
        col("Virtual disks", q("vmware_vm_disks")),
        col("Committed", q("vmware_vm_storage_committed_bytes"), unit="bytes"),
        col("Uncommitted (thin)", q("vmware_vm_storage_uncommitted_bytes"), unit="bytes"),
        col("Snapshots", q("vmware_vm_snapshots_existing"), thresholds=T_ORANGE),
        col("Oldest snapshot age", q("vmware_vm_snapshot_oldest_age_seconds"), unit="s", thresholds=T_SNAP_AGE),
    ], links={"vm_name": T_VM}, hide=("vm_moid",), filterable=True), 12, 8)
    d.add(table("Guest file systems (VMware Tools)", ["vcenter", "vm_name", "vm_moid", "disk_path"], [
        col("Capacity", q("vmware_vm_guest_disk_capacity_bytes"), unit="bytes"),
        col("Free", q("vmware_vm_guest_disk_free_bytes"), unit="bytes"),
        col("Free %", f"({m('vmware_vm_guest_disk_free_bytes', vs)} / {m('vmware_vm_guest_disk_capacity_bytes', vs)}){vf}", "pct", thresholds=T_FREE),
    ], hide=("vm_moid",), sort=("Free %", False), filterable=True), 12, 8)

    d.row("Network")
    d.add(ts("Network throughput", [perf("vmware_vm_perf_net_received_average_bytes_per_second", "{{vm_name}} rx"),
                                    perf("vmware_vm_perf_net_transmitted_average_bytes_per_second", "{{vm_name}} tx")], "Bps"), 12, 8)
    d.add(ts("Dropped packets (per 20 s sample)", [perf("vmware_vm_perf_net_dropped_rx_summation", "{{vm_name}} rx"),
                                                   perf("vmware_vm_perf_net_dropped_tx_summation", "{{vm_name}} tx")], "none"), 12, 8)

    d.row("Alarms")
    d.add(alarms_table(f'vmware_alarm_triggered{{{VC}, entity_type="VirtualMachine"}} and on(vcenter, entity) '
                       f'label_replace(vmware_vm_info{{{info}}}, "entity", "$1", "vm_name", "(.*)")', "Active alarms on the VM(s)"), 24, 7)
    return d


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    for build in (overview, clusters, hosts, vms):
        d = build()
        path = os.path.join(here, DASHBOARDS[d.key][3])
        with open(path, "w", encoding="utf-8") as f:
            json.dump(d.json(), f, indent=1, ensure_ascii=False)
            f.write("\n")
        print(f"wrote {path} ({len(d.panels)} panels)")


if __name__ == "__main__":
    main()
