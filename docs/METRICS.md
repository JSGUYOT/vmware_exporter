# Metrics reference

Every series carries the `vcenter` label (target name) plus the configured `extra_labels`.
vSphere statuses: 0 = gray/unknown, 1 = green, 2 = yellow, 3 = red. (*) = only appears on real infrastructure (active alarm, vSAN, appliance, snapshots...).

Native label names: `dc_name`, `cluster_name`, `host_name`, `ds_name`, `vm_name` (same names as the pryorda-compatible series below).

Every VM series (`vmware_vm_*`, including `vmware_vm_info` and `vmware_vm_perf_*`) carries `dc_name`, `folder_name`, `vm_name` and `vm_moid`. `folder_name` is the name of the VM's parent folder (`""` when the VM sits at the datacenter root VM folder or in a vApp). VM series do not carry `host_name`/`cluster_name` (so they stay stable across vMotion): see `vmware_vm_info`.

Real-time performance counters: `vmware_<host|vm>_perf_<group>_<counter>_<rollup>_<unit>` (e.g. `vmware_vm_perf_cpu_ready_summation_seconds`), label `perf_instance` (`""` = aggregate). Percentages as 0-100, bytes and seconds in base units.

## Exporter

Disabled by `collectors: {exporter: false}` (globally or per target), except `vmware_up`, always
emitted, and `vmware_exporter_build_info`, which describes the exporter process. The "Collection
per vCenter" section of the overview dashboard and the `VMwareExporter*` alerts based on these
metrics then stay empty.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_collection_duration_seconds` | gauge |  | Total duration of the last collection. |
| `vmware_collection_failures_total` | counter |  | Number of failed collections (vCenter unreachable / auth). |
| `vmware_collection_interval_seconds` | gauge |  | Configured collection interval. |
| `vmware_collection_timestamp_seconds` | gauge |  | Timestamp of the last collection. |
| `vmware_collection_timeout_seconds` | gauge |  | Configured collection timeout. |
| `vmware_collections_total` | counter |  | Number of collections performed. |
| `vmware_collector_duration_seconds` | gauge | collector | Collector duration during the last collection. |
| `vmware_collector_success` | gauge | collector | 1 if the collector succeeded during the last collection. |
| `vmware_exporter_build_info` | gauge | commit, govmomi_api, version | Exporter version (value always 1). |
| `vmware_exporter_duplicate_series` | gauge |  | Series dropped as duplicates (objects with the same name). |
| `vmware_exporter_invalid_series` | gauge |  | Invalid series dropped (e.g. label name conflict). |
| `vmware_exporter_missing_properties` | gauge |  | Properties unavailable on some objects during the last collection (permissions, unreachable ESX...). |
| `vmware_exporter_unsupported_properties` | gauge |  | Properties skipped because they do not exist in the vCenter's API version. |
| `vmware_up` | gauge |  | 1 if the last collection from the vCenter succeeded (connection + inventory). |

## vCenter

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_inventory_objects` | gauge | type | Number of inventory objects by type. |
| `vmware_vcenter_certificate_expiry_timestamp_seconds` | gauge |  | Expiry date of the vCenter TLS certificate (epoch). |
| `vmware_vcenter_info` | gauge | api_version_server, api_version_used, build, instance_uuid, product, product_line, version | vCenter information (value always 1). |
| `vmware_vcenter_sessions` | gauge |  | Number of open vCenter sessions (if the account is allowed to list them). |
| `vmware_vcenter_version_major` | gauge |  | vCenter major version (8 = vCenter 8, 9 = VCF/vSphere 9). |

## vCenter alarms

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_alarms_active` | gauge | status | Number of active vCenter alarms by severity. |
| `vmware_alarm_triggered` (*) | gauge | alarm, alarm_moid, entity, entity_type, entity_moid, dc_name, status, acknowledged | Active vCenter alarm on an entity, value = severity (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_alarm_triggered_timestamp_seconds` (*) | gauge | same as vmware_alarm_triggered | Time the vCenter alarm was triggered. |

## vCenter appliance (REST /api)

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_appliance_endpoint_up` | gauge | endpoint | 1 if the appliance REST endpoint responded (0 = not available on this version or insufficient permissions). |
| `vmware_appliance_backup_jobs` (*) | gauge |  | Number of known file-based backup jobs. |
| `vmware_appliance_backup_last_succeeded` (*) | gauge |  | 1 if the last vCenter backup succeeded. |
| `vmware_appliance_backup_last_success_timestamp_seconds` (*) | gauge |  | End time of the last successful vCenter backup. |
| `vmware_appliance_health` (*) | gauge | component | Health of a vCenter appliance component (0=gray/unknown, 1=green, 2=yellow, 3=red) (orange counted as 2). |
| `vmware_appliance_info` (*) | gauge | version, build, product, type | vCenter appliance version (value always 1). |
| `vmware_appliance_service_health` (*) | gauge | service, startup_type | vCenter service health (0=unknown, 1=healthy, 2=warnings, 3=degraded). |
| `vmware_appliance_service_started` (*) | gauge | service, startup_type | 1 if the vCenter service is started. |
| `vmware_appliance_uptime_seconds` (*) | gauge |  | vCenter appliance uptime. |

## Datacenter

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_datacenter_overall_status` | gauge | dc_name | Overall datacenter status (0=gray/unknown, 1=green, 2=yellow, 3=red). |

## Cluster

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_cluster_config_issues` | gauge | cluster_name, dc_name | Number of configuration issues reported on the cluster. |
| `vmware_cluster_cpu_cores` | gauge | cluster_name, dc_name | Number of physical cores. |
| `vmware_cluster_cpu_demand_mhz` | gauge | cluster_name, dc_name | VM CPU demand (MHz). |
| `vmware_cluster_cpu_effective_mhz` | gauge | cluster_name, dc_name | Effective CPU capacity available to VMs (MHz). |
| `vmware_cluster_cpu_entitled_mhz` | gauge | cluster_name, dc_name | CPU entitled to VMs (MHz). |
| `vmware_cluster_cpu_reserved_mhz` | gauge | cluster_name, dc_name | Reserved CPU (MHz). |
| `vmware_cluster_cpu_threads` | gauge | cluster_name, dc_name | Number of physical threads. |
| `vmware_cluster_cpu_total_mhz` | gauge | cluster_name, dc_name | Total cluster CPU capacity (MHz). |
| `vmware_cluster_datastores` | gauge | cluster_name, dc_name | Number of datastores mounted on the cluster. |
| `vmware_cluster_drs_enabled` | gauge | cluster_name, dc_name | 1 if DRS is enabled. |
| `vmware_cluster_drs_fully_automated` | gauge | cluster_name, dc_name | 1 if DRS is in fully automated mode. |
| `vmware_cluster_drs_recommendations` | gauge | cluster_name, dc_name | Number of pending DRS recommendations. |
| `vmware_cluster_ha_admission_control_enabled` | gauge | cluster_name, dc_name | 1 if HA admission control is enabled. |
| `vmware_cluster_ha_admission_control_policy` | gauge | cluster_name, dc_name, policy | HA admission control policy (policy label, value 1). |
| `vmware_cluster_ha_enabled` | gauge | cluster_name, dc_name | 1 if vSphere HA is enabled. |
| `vmware_cluster_ha_host_monitoring_enabled` | gauge | cluster_name, dc_name | 1 if HA host monitoring is enabled. |
| `vmware_cluster_hosts` | gauge | cluster_name, dc_name | Number of ESX hosts in the cluster. |
| `vmware_cluster_hosts_effective` | gauge | cluster_name, dc_name | Number of effective ESX hosts (connected, not in maintenance). |
| `vmware_cluster_memory_demand_bytes` | gauge | cluster_name, dc_name | VM memory demand. |
| `vmware_cluster_memory_effective_bytes` | gauge | cluster_name, dc_name | Effective memory available to VMs. |
| `vmware_cluster_memory_entitled_bytes` | gauge | cluster_name, dc_name | Memory entitled to VMs. |
| `vmware_cluster_memory_reserved_bytes` | gauge | cluster_name, dc_name | Reserved memory. |
| `vmware_cluster_memory_total_bytes` | gauge | cluster_name, dc_name | Total cluster memory. |
| `vmware_cluster_overall_status` | gauge | cluster_name, dc_name | Overall cluster status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_cluster_vmotions_total` | counter | cluster_name, dc_name | Number of vMotions performed in the cluster. |
| `vmware_cluster_vms` | gauge | cluster_name, dc_name | Total number of VMs. |
| `vmware_cluster_vms_powered_on` | gauge | cluster_name, dc_name | Number of powered-on VMs. |
| `vmware_cluster_vsan_enabled` | gauge | cluster_name, dc_name | 1 if vSAN is enabled on the cluster. |
| `vmware_cluster_dpm_enabled` (*) | gauge | dc_name, cluster_name | 1 if DPM (power management) is enabled. |
| `vmware_cluster_drs_score` (*) | gauge | dc_name, cluster_name | Cluster DRS score (0-100, VM happiness). |
| `vmware_cluster_ha_failover_cpu_resources_configured_percent` (*) | gauge | dc_name, cluster_name | Percentage of CPU reserved for HA failover (configured or computed). |
| `vmware_cluster_ha_failover_cpu_resources_percent` (*) | gauge | dc_name, cluster_name | Available HA failover CPU resources (%). |
| `vmware_cluster_ha_failover_level_configured` (*) | gauge | dc_name, cluster_name | Configured number of ESX host failures to tolerate ('host failures' policy). |
| `vmware_cluster_ha_failover_level_current` (*) | gauge | dc_name, cluster_name | Number of ESX host failures currently tolerated ('host failures' policy, slots). |
| `vmware_cluster_ha_failover_memory_resources_configured_percent` (*) | gauge | dc_name, cluster_name | Percentage of memory reserved for HA failover (configured or computed). |
| `vmware_cluster_ha_failover_memory_resources_percent` (*) | gauge | dc_name, cluster_name | Available HA failover memory resources (%). |

## ESX

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_host_boot_time_seconds` | gauge | cluster_name, dc_name, host_name | Timestamp of the last ESX boot. |
| `vmware_host_certificate_expiry_timestamp_seconds` | gauge | cluster_name, dc_name, host_name | Expiry date of the ESX certificate (epoch). |
| `vmware_host_config_issues` | gauge | cluster_name, dc_name, host_name | Number of configuration issues reported on the ESX host. |
| `vmware_host_connection_status` | gauge | cluster_name, dc_name, host_name | ESX connection state to vCenter (0=disconnected, 1=connected, 2=notResponding). |
| `vmware_host_cpu_cores` | gauge | cluster_name, dc_name, host_name | Number of physical cores. |
| `vmware_host_cpu_threads` | gauge | cluster_name, dc_name, host_name | Number of physical threads. |
| `vmware_host_cpu_total_mhz` | gauge | cluster_name, dc_name, host_name | Total CPU capacity (MHz). |
| `vmware_host_cpu_usage_mhz` | gauge | cluster_name, dc_name, host_name | CPU used (MHz). |
| `vmware_host_datastores` | gauge | cluster_name, dc_name, host_name | Number of datastores mounted on the ESX host. |
| `vmware_host_hardware_status` | gauge | cluster_name, component, dc_name, host_name, name | Status of a hardware component (memory, cpu, storage) (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_info` | gauge | bios_version, build, cluster_name, cpu_model, dc_name, host_name, model, product, vendor, version | ESX host information (value always 1). |
| `vmware_host_in_maintenance` | gauge | cluster_name, dc_name, host_name | 1 if the ESX host is in maintenance mode. |
| `vmware_host_memory_total_bytes` | gauge | cluster_name, dc_name, host_name | Total physical memory. |
| `vmware_host_memory_usage_bytes` | gauge | cluster_name, dc_name, host_name | Memory used. |
| `vmware_host_overall_status` | gauge | cluster_name, dc_name, host_name | Overall ESX status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_pnic_link_up` | gauge | cluster_name, dc_name, driver, host_name, nic, used | 1 if the physical NIC link is up. used=true if it is an uplink of a vSwitch/vDS. |
| `vmware_host_pnic_speed_mbps` | gauge | cluster_name, dc_name, driver, host_name, nic, used | Negotiated physical NIC speed (Mb/s, 0 if link is down). |
| `vmware_host_power_status` | gauge | cluster_name, dc_name, host_name | ESX power state (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_host_quarantine_mode` | gauge | cluster_name, dc_name, host_name | 1 if the ESX host is in quarantine mode (Proactive HA). |
| `vmware_host_reboot_required` | gauge | cluster_name, dc_name, host_name | 1 if an ESX reboot is required. |
| `vmware_host_sensor_health` | gauge | cluster_name, dc_name, host_name, sensor, type | Health of an IPMI/CIM hardware sensor (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_sensor_value` | gauge | cluster_name, dc_name, host_name, sensor, type, unit | Current value of a hardware sensor (unit in the unit label). |
| `vmware_host_service_running` | gauge | cluster_name, dc_name, host_name, policy, service | 1 if the ESX service is running (TSM-SSH, ntpd, ...). |
| `vmware_host_storage_luns` | gauge | cluster_name, dc_name, host_name | Number of LUNs seen by the ESX host. |
| `vmware_host_storage_luns_degraded` | gauge | cluster_name, dc_name, host_name | Multipath LUNs with only one live path left (redundancy lost). |
| `vmware_host_storage_luns_no_path` | gauge | cluster_name, dc_name, host_name | LUNs with no live path at all (access lost). |
| `vmware_host_storage_paths` | gauge | cluster_name, dc_name, host_name, state | Number of storage paths by state (active, standby, dead...). |
| `vmware_host_uptime_seconds` | gauge | cluster_name, dc_name, host_name | ESX host uptime. |
| `vmware_host_vms` | gauge | cluster_name, dc_name, host_name | Number of VMs registered on the ESX host. |
| `vmware_host_ha_state` (*) | gauge | dc_name, cluster_name, host_name, state | State of the ESX HA agent (FDM), value 1 for the current state. |

## Virtual machines

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_vm_connection_state` | gauge | dc_name, folder_name, vm_name, vm_moid | VM connection state (0=connected, 1=disconnected, 2=orphaned, 3=inaccessible, 4=invalid). |
| `vmware_vm_consolidation_needed` | gauge | dc_name, folder_name, vm_name, vm_moid | 1 if the VM disks need consolidation. |
| `vmware_vm_cpu_demand_mhz` | gauge | dc_name, folder_name, vm_name, vm_moid | CPU demand (MHz). |
| `vmware_vm_cpu_max_mhz` | gauge | dc_name, folder_name, vm_name, vm_moid | Maximum usable CPU (MHz). |
| `vmware_vm_cpu_usage_mhz` | gauge | dc_name, folder_name, vm_name, vm_moid | CPU used (MHz). |
| `vmware_vm_disks` | gauge | dc_name, folder_name, vm_name, vm_moid | Number of virtual disks. |
| `vmware_vm_guest_heartbeat_status` | gauge | dc_name, folder_name, vm_name, vm_moid | Guest OS VMware Tools heartbeat (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vm_info` | gauge | cluster_name, dc_name, folder_name, guest_hostname, guest_os, host_name, hw_version, ip_address, managed_by, template, tools_version, vm_name, vm_moid | VM information (value always 1). Carries the current cluster and host. |
| `vmware_vm_memory_ballooned_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Memory reclaimed by ballooning. |
| `vmware_vm_memory_compressed_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Compressed memory. |
| `vmware_vm_memory_guest_active_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Guest active memory. |
| `vmware_vm_memory_host_consumed_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Memory consumed on the ESX host. |
| `vmware_vm_memory_size_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Configured memory. |
| `vmware_vm_memory_swapped_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Memory swapped by the hypervisor. |
| `vmware_vm_nics` | gauge | dc_name, folder_name, vm_name, vm_moid | Number of virtual NICs. |
| `vmware_vm_overall_status` | gauge | dc_name, folder_name, vm_name, vm_moid | Overall VM status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vm_power_status` | gauge | dc_name, folder_name, vm_name, vm_moid | VM power state (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_vm_question_pending` | gauge | dc_name, folder_name, vm_name, vm_moid | 1 if the VM is blocked on a question awaiting an answer. |
| `vmware_vm_snapshots_existing` | gauge | dc_name, folder_name, vm_name, vm_moid | Number of VM snapshots. |
| `vmware_vm_storage_committed_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Storage actually consumed on datastores. |
| `vmware_vm_storage_uncommitted_bytes` | gauge | dc_name, folder_name, vm_name, vm_moid | Provisioned storage not yet consumed (thin). |
| `vmware_vm_tools_running` | gauge | dc_name, folder_name, vm_name, vm_moid | 1 if VMware Tools is running in the guest. |
| `vmware_vm_tools_version_status` | gauge | dc_name, folder_name, status, vm_name, vm_moid | VMware Tools version status (status label, value 1). |
| `vmware_vm_uptime_seconds` | gauge | dc_name, folder_name, vm_name, vm_moid | VM uptime. |
| `vmware_vm_vcpus` | gauge | dc_name, folder_name, vm_name, vm_moid | Number of vCPUs. |
| `vmware_vms_monitored` | gauge |  | Number of monitored VMs (after filters). |
| `vmware_vms_powered_on` | gauge |  | Number of monitored VMs that are powered on. |
| `vmware_vms_system_managed` | gauge |  | Number of VMs managed by a solution (vCLS, Supervisor, agents...). |
| `vmware_vms_templates` | gauge |  | Number of templates in the inventory. |
| `vmware_vm_boot_time_seconds` (*) | gauge | dc_name, folder_name, vm_name, vm_moid | Timestamp of the last VM boot. |
| `vmware_vm_guest_disk_capacity_bytes` (*) | gauge | dc_name, folder_name, vm_name, vm_moid, disk_path | Capacity of a guest file system (via VMware Tools). |
| `vmware_vm_guest_disk_free_bytes` (*) | gauge | dc_name, folder_name, vm_name, vm_moid, disk_path | Free space on a guest file system (via VMware Tools). |
| `vmware_vm_ha_protected` (*) | gauge | dc_name, folder_name, vm_name, vm_moid | 1 if the VM is protected by vSphere HA. |
| `vmware_vm_snapshot_oldest_age_seconds` (*) | gauge | dc_name, folder_name, vm_name, vm_moid | Age of the VM's oldest snapshot. |
| `vmware_vm_snapshot_oldest_timestamp_seconds` (*) | gauge | dc_name, folder_name, vm_name, vm_moid | Creation date of the oldest snapshot. |

## Datastores

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_datastore_is_accessible` | gauge | dc_name, ds_name, datastore_moid | 1 if the datastore is accessible. |
| `vmware_datastore_capacity_bytes` | gauge | dc_name, ds_name, datastore_moid | Datastore capacity. |
| `vmware_datastore_cluster_capacity_bytes` | gauge | dc_name, datastore_cluster | Datastore cluster capacity. |
| `vmware_datastore_cluster_datastores` | gauge | dc_name, datastore_cluster | Number of datastores in the datastore cluster. |
| `vmware_datastore_cluster_free_bytes` | gauge | dc_name, datastore_cluster | Datastore cluster free space. |
| `vmware_datastore_cluster_overall_status` | gauge | dc_name, datastore_cluster | Overall datastore cluster status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_free_bytes` | gauge | dc_name, ds_name, datastore_moid | Datastore free space. |
| `vmware_datastore_hosts_inaccessible` | gauge | dc_name, ds_name, datastore_moid | Number of ESX hosts for which the datastore is inaccessible (APD/PDL...). |
| `vmware_datastore_hosts_mounted` | gauge | dc_name, ds_name, datastore_moid | Number of ESX hosts mounting the datastore. |
| `vmware_datastore_info` | gauge | dc_name, ds_name, datastore_cluster, datastore_moid, shared, type, url | Datastore information (value always 1). |
| `vmware_datastore_in_maintenance` | gauge | dc_name, ds_name, datastore_moid | 1 if the datastore is in (or entering) maintenance mode. |
| `vmware_datastore_overall_status` | gauge | dc_name, ds_name, datastore_moid | Overall datastore status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_provisioned_bytes` | gauge | dc_name, ds_name, datastore_moid | Total provisioned space (used + unconsumed thin). |
| `vmware_datastore_uncommitted_bytes` | gauge | dc_name, ds_name, datastore_moid | Provisioned but unconsumed space (thin) on the datastore. |
| `vmware_datastore_vms_hosted` | gauge | dc_name, ds_name, datastore_moid | Number of VMs with files on the datastore. |

## vSphere Distributed Switch

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_dvs_hosts` | gauge | dc_name, dvs | Number of ESX hosts that are members of the vDS. |
| `vmware_dvs_info` | gauge | dc_name, dvs, version | vDS information (value always 1). |
| `vmware_dvs_overall_status` | gauge | dc_name, dvs | Overall vDS status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_dvs_ports` | gauge | dc_name, dvs | Number of vDS ports. |
| `vmware_dvs_host_member_up` (*) | gauge | dc_name, dvs, host_name | 1 if the ESX member of the vDS is 'up' (otherwise outOfSync, warning, down...). |
| `vmware_dvs_host_members` (*) | gauge | dc_name, dvs, status | Number of ESX hosts that are members of the vDS, by status. |

## Networks / portgroups

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_network_accessible` | gauge | dc_name, network, network_moid, network_type | 1 if the network is accessible from at least one ESX host. |
| `vmware_network_hosts` | gauge | dc_name, network, network_moid, network_type | Number of ESX hosts with access to the network. |
| `vmware_network_overall_status` | gauge | dc_name, network, network_moid, network_type | Overall network / portgroup status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_network_vms` | gauge | dc_name, network, network_moid, network_type | Number of VMs connected to the network. |

## Resource pools

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_resourcepool_cpu_limit_mhz` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Pool CPU limit (MHz, -1 = unlimited). |
| `vmware_resourcepool_cpu_reservation_mhz` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Pool CPU reservation (MHz). |
| `vmware_resourcepool_memory_limit_bytes` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Pool memory limit (-1 = unlimited). |
| `vmware_resourcepool_memory_reservation_bytes` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Pool memory reservation. |
| `vmware_resourcepool_overall_status` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Overall resource pool status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_resourcepool_vms` | gauge | cluster_name, dc_name, resource_pool, resource_pool_moid | Number of VMs directly in the resource pool. |
| `vmware_resourcepool_cpu_usage_mhz` (*) | gauge | dc_name, cluster_name, resource_pool, resource_pool_moid | CPU used by the resource pool (MHz). |
| `vmware_resourcepool_memory_ballooned_bytes` (*) | gauge | dc_name, cluster_name, resource_pool, resource_pool_moid | Ballooned memory in the pool. |
| `vmware_resourcepool_memory_guest_active_bytes` (*) | gauge | dc_name, cluster_name, resource_pool, resource_pool_moid | Active guest memory in the pool. |
| `vmware_resourcepool_memory_host_consumed_bytes` (*) | gauge | dc_name, cluster_name, resource_pool, resource_pool_moid | Memory consumed on ESX hosts by the pool. |
| `vmware_resourcepool_memory_swapped_bytes` (*) | gauge | dc_name, cluster_name, resource_pool, resource_pool_moid | Swapped memory in the pool. |

## vSAN

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_vsan_cluster_health` (*) | gauge | dc_name, cluster_name | Overall vSAN health of the cluster (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_group` (*) | gauge | dc_name, cluster_name, group | Health of a vSAN test group (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_query_success` (*) | gauge | dc_name, cluster_name | 1 if the vSAN health summary could be read for the cluster. |
| `vmware_vsan_health_test` (*) | gauge | dc_name, cluster_name, group, test | Result of a vSAN health test (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_timestamp_seconds` (*) | gauge | dc_name, cluster_name | Timestamp of the last vSAN health test. |

## pryorda/vmware_exporter compatibility (optional)

Only with `collectors: {pryorda: true}`; see [Migrating from pryorda/vmware_exporter](../README.md#migrating-from-pryordavmware_exporter).
All gauges; names, labels, units and values identical to pryorda/vmware_exporter 0.18.
Base labels (in addition to `vcenter` / `extra_labels`):
VM = `vm_name, ds_name, host_name, dc_name, cluster_name`; host = `host_name, dc_name, cluster_name`; datastore = `ds_name, dc_name, ds_cluster`.

| Metric | Extra labels | Description |
|---|---|---|
| `vmware_vm_power_state` | | 1 = poweredOn, 0 otherwise |
| `vmware_vm_boot_timestamp_seconds` | | boot time (powered-on VMs) |
| `vmware_vm_num_cpu` | | vCPUs |
| `vmware_vm_memory_max` | | configured memory (MB) |
| `vmware_vm_max_cpu_usage` | | maximum usable CPU (MHz) |
| `vmware_vm_template` | | 1 if template |
| `vmware_vm_guest_disk_free` / `_capacity` | `partition` | guest filesystem free / capacity (bytes) |
| `vmware_vm_guest_tools_running_status` | `tools_status` | always 1 |
| `vmware_vm_guest_tools_version` | `tools_version` | always 1 |
| `vmware_vm_guest_tools_version_status` | `tools_version_status` | always 1 |
| `vmware_vm_snapshots` (*) | | number of snapshots (absent when none) |
| `vmware_vm_snapshot_timestamp_seconds` (*) | `vm_snapshot_name` | snapshot creation time |
| `vmware_host_power_state` | | 1 = poweredOn |
| `vmware_host_standby_mode` | `standby_mode_state` | 1 if in standby |
| `vmware_host_connection_state` | `state` | always 1 |
| `vmware_host_maintenance_mode` | | 1 if in maintenance |
| `vmware_host_boot_timestamp_seconds` | | boot time (powered-on hosts, as all below) |
| `vmware_host_cpu_usage` / `_cpu_max` | | MHz |
| `vmware_host_num_cpu` | | physical cores |
| `vmware_host_memory_usage` / `_memory_max` | | MB |
| `vmware_host_product_info` | `version, build` | always 1 |
| `vmware_host_hardware_info` | `hardware_model, hardware_cpu_model` | always 1 |
| `vmware_host_sensor_state` | `name, type` | 0=red, 1=yellow, 2=green, 3=unknown |
| `vmware_host_sensor_fan` / `_temperature` / `_power_voltage` / `_power_current` / `_power_watt` / `_redundancy` | `name` | sensor reading |
| `vmware_datastore_capacity_size` / `_freespace_size` / `_uncommited_size` / `_provisoned_size` | | bytes |
| `vmware_datastore_hosts` / `_vms` | | mounting hosts / VMs |
| `vmware_datastore_maintenance_mode` | `mode` | always 1 |
| `vmware_datastore_type` | `ds_type` | always 1 |
| `vmware_datastore_accessible` | | 1 if accessible |
| `vmware_vm_<group>_<counter>_<rollup>` | | 24 real-time counters, raw vSphere units: cpu ready/maxlimited/usage/usagemhz/costop/idle/demand, mem usage/consumed/active/swapped/vmmemctl, disk maxTotalLatency/usage/read/write, net received/transmitted/multicastRx/multicastTx/broadcastRx/broadcastTx/droppedRx/droppedTx |
| `vmware_host_<group>_<counter>_<rollup>` | | 24 real-time counters, raw vSphere units: cpu costop/demand/idle/ready/swapwait/usage/usagemhz/used, disk read/write, mem active/latency/swapin/swapinRate/swapout/swapoutRate/vmmemctl, net bytesRx/bytesTx/droppedRx/droppedTx/errorsRx/errorsTx/usage |
