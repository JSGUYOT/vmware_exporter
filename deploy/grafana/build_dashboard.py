#!/usr/bin/env python3
"""Génère deploy/grafana/vmware-overview.json (tableau de bord Grafana >= 10).

Régénération : python3 build_dashboard.py > vmware-overview.json
"""
import json

DS = {"type": "prometheus", "uid": "${datasource}"}
SEL = 'vcenter=~"$vcenter", datacenter=~"$datacenter"'
SELC = SEL + ', cluster=~"$cluster"'
# Les séries VM ne portent pas cluster/host (stables au vMotion) : filtre via vmware_vm_info.
VMF = ' and on(vcenter, vm_moid) vmware_vm_info{' + SELC + '}'

panels = []
pid = [0]


def nid():
    pid[0] += 1
    return pid[0]


def pos(x, y, w, h):
    return {"x": x, "y": y, "w": w, "h": h}


def row(title, y):
    panels.append({"type": "row", "title": title, "id": nid(), "gridPos": pos(0, y, 24, 1), "collapsed": False, "panels": []})


def stat(title, expr, x, y, w=4, h=4, unit="none", thresholds=None, color_mode="background", desc=""):
    steps = thresholds or [{"color": "green", "value": None}]
    panels.append({
        "type": "stat", "title": title, "id": nid(), "datasource": DS, "gridPos": pos(x, y, w, h), "description": desc,
        "targets": [{"refId": "A", "expr": expr, "datasource": DS}],
        "fieldConfig": {"defaults": {"unit": unit, "thresholds": {"mode": "absolute", "steps": steps}}, "overrides": []},
        "options": {"colorMode": color_mode, "graphMode": "none", "reduceOptions": {"calcs": ["lastNotNull"]}, "textMode": "value"},
    })


def ts(title, targets, x, y, w=12, h=8, unit="none", desc="", max_=None):
    d = {"unit": unit, "custom": {"lineWidth": 1, "fillOpacity": 10, "showPoints": "never"}}
    if max_ is not None:
        d["max"] = max_
    panels.append({
        "type": "timeseries", "title": title, "id": nid(), "datasource": DS, "gridPos": pos(x, y, w, h), "description": desc,
        "targets": [{"refId": chr(65 + i), "expr": e, "legendFormat": l, "datasource": DS} for i, (e, l) in enumerate(targets)],
        "fieldConfig": {"defaults": d, "overrides": []},
        "options": {"legend": {"displayMode": "table", "placement": "right", "calcs": ["lastNotNull", "max"]}, "tooltip": {"mode": "multi", "sort": "desc"}},
    })


def table(title, expr, x, y, w, h, columns, desc="", value_name="Valeur", unit="none", sort=None):
    org = {"excludeByName": {"Time": True, "__name__": True, "job": True, "instance": True}, "renameByName": {"Value": value_name}, "indexByName": {}}
    for i, c in enumerate(columns):
        org["indexByName"][c] = i
    p = {
        "type": "table", "title": title, "id": nid(), "datasource": DS, "gridPos": pos(x, y, w, h), "description": desc,
        "targets": [{"refId": "A", "expr": expr, "format": "table", "instant": True, "datasource": DS}],
        "transformations": [{"id": "organize", "options": org}],
        "fieldConfig": {"defaults": {"unit": unit}, "overrides": []},
        "options": {"showHeader": True, "cellHeight": "sm"},
    }
    if sort:
        p["options"]["sortBy"] = [{"displayName": sort, "desc": True}]
    panels.append(p)


def bargauge(title, expr, legend, x, y, w, h, unit="percentunit"):
    panels.append({
        "type": "bargauge", "title": title, "id": nid(), "datasource": DS, "gridPos": pos(x, y, w, h),
        "targets": [{"refId": "A", "expr": expr, "legendFormat": legend, "instant": True, "datasource": DS}],
        "fieldConfig": {"defaults": {"unit": unit, "min": 0, "max": 1, "thresholds": {"mode": "absolute", "steps": [
            {"color": "green", "value": None}, {"color": "orange", "value": 0.85}, {"color": "red", "value": 0.95}]}}, "overrides": []},
        "options": {"displayMode": "gradient", "orientation": "horizontal", "showUnfilled": True, "reduceOptions": {"calcs": ["lastNotNull"]}},
    })


RED = [{"color": "green", "value": None}, {"color": "red", "value": 1}]
ORANGE = [{"color": "green", "value": None}, {"color": "orange", "value": 1}]

y = 0
row("Vue d'ensemble", y); y += 1
stat("vCenter joignables", 'sum(vmware_up{vcenter=~"$vcenter"}) / count(vmware_up{vcenter=~"$vcenter"})', 0, y, unit="percentunit",
     thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 1}])
stat("Alarmes rouges", 'count(vmware_alarm_triggered{' + SEL + ', status="red"}) or vector(0)', 4, y, thresholds=RED)
stat("Alarmes jaunes", 'count(vmware_alarm_triggered{' + SEL + ', status="yellow"}) or vector(0)', 8, y, thresholds=ORANGE)
stat("ESX connectés", 'count(vmware_host_connection_state{' + SELC + '} == 1) or vector(0)', 12, y, color_mode="value")
stat("ESX en défaut", 'count(vmware_host_connection_state{' + SELC + '} != 1) or vector(0)', 16, y, thresholds=RED)
stat("VMs allumées", 'count((vmware_vm_power_state{' + SEL + '} == 1)' + VMF + ') or vector(0)', 20, y, color_mode="value")
y += 4
stat("Datastores > 85 %", 'count((1 - vmware_datastore_free_bytes{' + SEL + '} / vmware_datastore_capacity_bytes{' + SEL + '}) > 0.85) or vector(0)', 0, y, thresholds=ORANGE)
stat("Snapshots > 3 j", 'count((vmware_vm_snapshot_oldest_age_seconds{' + SEL + '} > 259200)' + VMF + ') or vector(0)', 4, y, thresholds=ORANGE)
stat("VMs sans Tools", 'count((vmware_vm_tools_running{' + SEL + '} == 0) and on(vcenter, vm_moid) (vmware_vm_power_state == 1)' + VMF + ') or vector(0)', 8, y, thresholds=ORANGE)
stat("Chemins stockage morts", 'sum(vmware_host_storage_paths{' + SELC + ', state="dead"}) or vector(0)', 12, y, thresholds=RED)
stat("Santé vSAN (pire)", 'max(vmware_vsan_cluster_health{' + SELC + '}) or vector(0)', 16, y,
     thresholds=[{"color": "text", "value": None}, {"color": "green", "value": 1}, {"color": "orange", "value": 2}, {"color": "red", "value": 3}],
     desc="0 = pas de vSAN / inconnu, 1 = vert, 2 = jaune, 3 = rouge")
stat("Collecteurs en échec", 'count(vmware_collector_success{vcenter=~"$vcenter"} == 0) or vector(0)', 20, y, thresholds=ORANGE)
y += 4

row("Alarmes vCenter actives", y); y += 1
table("Alarmes vCenter actives", 'vmware_alarm_triggered{' + SEL + '}', 0, y, 24, 8,
      ["vcenter", "datacenter", "status", "alarm", "entity_type", "entity", "acknowledged"], value_name="Sévérité")
y += 8

row("Clusters", y); y += 1
ts("CPU : demande / capacité effective", [('vmware_cluster_cpu_demand_mhz{' + SELC + '} / vmware_cluster_cpu_effective_mhz{' + SELC + '}', "{{vcenter}} / {{cluster}}")], 0, y, unit="percentunit")
ts("Mémoire : demande / capacité effective", [('vmware_cluster_memory_demand_bytes{' + SELC + '} / vmware_cluster_memory_effective_bytes{' + SELC + '}', "{{vcenter}} / {{cluster}}")], 12, y, unit="percentunit")
y += 8
table("État des clusters", 'vmware_cluster_hosts_effective{' + SELC + '}', 0, y, 12, 7, ["vcenter", "datacenter", "cluster"], value_name="ESX effectifs")
ts("Score DRS", [('vmware_cluster_drs_score{' + SELC + '}', "{{cluster}}")], 12, y, w=12, h=7, unit="none", max_=100)
y += 7

row("ESX", y); y += 1
ts("CPU par ESX", [('vmware_host_cpu_usage_mhz{' + SELC + '} / vmware_host_cpu_total_mhz{' + SELC + '}', "{{host}}")], 0, y, unit="percentunit")
ts("Mémoire par ESX", [('vmware_host_memory_usage_bytes{' + SELC + '} / vmware_host_memory_total_bytes{' + SELC + '}', "{{host}}")], 12, y, unit="percentunit")
y += 8
ts("Latence datastore max par ESX", [('max by (host) (vmware_host_perf_disk_max_total_latency_latest_seconds{' + SELC + '})', "{{host}}")], 0, y, unit="s")
ts("Réseau par ESX (rx+tx)", [('vmware_host_perf_net_received_average_bytes_per_second{' + SELC + ', perf_instance=""} + vmware_host_perf_net_transmitted_average_bytes_per_second{' + SELC + ', perf_instance=""}', "{{host}}")], 12, y, unit="Bps")
y += 8
table("Inventaire ESX", 'vmware_host_info{' + SELC + '}', 0, y, 24, 8, ["vcenter", "datacenter", "cluster", "host", "version", "build", "vendor", "model", "cpu_model", "bios_version"], value_name="-")
y += 8

row("Datastores", y); y += 1
bargauge("Remplissage des datastores", '1 - vmware_datastore_free_bytes{' + SEL + '} / vmware_datastore_capacity_bytes{' + SEL + '}', "{{datastore}}", 0, y, 12, 10)
ts("Espace libre", [('vmware_datastore_free_bytes{' + SEL + '}', "{{datastore}}")], 12, y, h=10, unit="bytes")
y += 10

row("Machines virtuelles", y); y += 1
ts("Top 10 CPU ready (% par vCPU)", [('topk(10, ((vmware_vm_perf_cpu_ready_summation_seconds{' + SEL + ', perf_instance=""} / 20) / ignoring(perf_instance) vmware_vm_vcpus{' + SEL + '})' + VMF + ')', "{{vm}}")], 0, y, unit="percentunit")
ts("Top 10 latence disque", [('topk(10, vmware_vm_perf_disk_max_total_latency_latest_seconds{' + SEL + '}' + VMF + ')', "{{vm}}")], 12, y, unit="s")
y += 8
ts("Top 10 CPU consommé (MHz)", [('topk(10, vmware_vm_cpu_usage_mhz{' + SEL + '}' + VMF + ')', "{{vm}}")], 0, y, unit="none")
ts("Mémoire swappée / ballooning", [('sum by (vm) (vmware_vm_memory_swapped_bytes{' + SEL + '}' + VMF + ') > 0', "swap {{vm}}"), ('sum by (vm) (vmware_vm_memory_ballooned_bytes{' + SEL + '}' + VMF + ') > 0', "balloon {{vm}}")], 12, y, unit="bytes")
y += 8
table("Snapshots", 'vmware_vm_snapshot_oldest_age_seconds{' + SEL + '}' + VMF, 0, y, 12, 8, ["vcenter", "datacenter", "vm"], value_name="Âge du plus ancien", unit="s", sort="Âge du plus ancien")
table("Disques invités < 15 % libres", '((vmware_vm_guest_disk_free_bytes{' + SEL + '} / vmware_vm_guest_disk_capacity_bytes{' + SEL + '}) < 0.15)' + VMF, 12, y, 12, 8, ["vcenter", "vm", "disk_path"], value_name="Libre", unit="percentunit")
y += 8

row("vSAN", y); y += 1
table("Tests de santé vSAN non verts", 'vmware_vsan_health_test{' + SELC + '} > 1', 0, y, 24, 8, ["vcenter", "cluster", "group", "test"], value_name="Sévérité")
y += 8

row("Exporter", y); y += 1
ts("Durée de collecte", [('vmware_collection_duration_seconds{vcenter=~"$vcenter"}', "{{vcenter}}")], 0, y, unit="s")
table("Collecteurs", 'vmware_collector_success{vcenter=~"$vcenter"}', 12, y, 12, 8, ["vcenter", "collector"], value_name="OK")


def var(name, label, query, multi=True):
    return {
        "name": name, "label": label, "type": "query", "datasource": DS, "refresh": 2, "includeAll": True, "multi": multi,
        "allValue": ".*", "current": {"selected": True, "text": "All", "value": "$__all"},
        "query": {"query": query, "refId": name}, "definition": query, "sort": 1,
    }


dash = {
    "title": "VMware vSphere / VCF - Vue d'ensemble",
    "uid": "vmware-exporter-overview",
    "tags": ["vmware", "vsphere", "vcf"],
    "timezone": "browser",
    "schemaVersion": 39,
    "version": 1,
    "refresh": "1m",
    "time": {"from": "now-6h", "to": "now"},
    "templating": {"list": [
        {"name": "datasource", "label": "Source", "type": "datasource", "query": "prometheus", "current": {}, "refresh": 1},
        var("vcenter", "vCenter", 'label_values(vmware_up, vcenter)'),
        var("datacenter", "Datacenter", 'label_values(vmware_datacenter_overall_status{vcenter=~"$vcenter"}, datacenter)'),
        var("cluster", "Cluster", 'label_values(vmware_cluster_overall_status{vcenter=~"$vcenter", datacenter=~"$datacenter"}, cluster)'),
    ]},
    "panels": panels,
    "annotations": {"list": []},
}
print(json.dumps(dash, indent=1, ensure_ascii=False))
