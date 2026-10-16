# Metrics reference

Every series carries the `vcenter` label (target name) plus the configured `extra_labels`.
vSphere statuses: 0 = gray/unknown, 1 = green, 2 = yellow, 3 = red. (*) = only appears on real infrastructure (active alarm, vSAN, appliance, snapshots...).

VM series do not carry `host`/`cluster` (so they stay stable across vMotion): see `vmware_vm_info`.

Real-time performance counters: `vmware_<host|vm>_perf_<group>_<counter>_<rollup>_<unit>` (e.g. `vmware_vm_perf_cpu_ready_summation_seconds`), label `perf_instance` (`""` = aggregate). Percentages as 0-100, bytes and seconds in base units.

## Exporter

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_collection_duration_seconds` | gauge |  | Total duration of the last collection. |
| `vmware_collection_failures_total` | counter |  | Number of failed collections (vCenter unreachable / auth). |
| `vmware_collection_timestamp_seconds` | gauge |  | Timestamp of the last collection. |
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
| `vmware_alarm_triggered` (*) | gauge | alarm, alarm_moid, entity, entity_type, entity_moid, datacenter, status, acknowledged | Active vCenter alarm on an entity, value = severity (0=gray/unknown, 1=green, 2=yellow, 3=red). |
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
| `vmware_datacenter_overall_status` | gauge | datacenter | Overall datacenter status (0=gray/unknown, 1=green, 2=yellow, 3=red). |

## Cluster

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_cluster_config_issues` | gauge | cluster, datacenter | Number of configuration issues reported on the cluster. |
| `vmware_cluster_cpu_cores` | gauge | cluster, datacenter | Number of physical cores. |
| `vmware_cluster_cpu_demand_mhz` | gauge | cluster, datacenter | VM CPU demand (MHz). |
| `vmware_cluster_cpu_effective_mhz` | gauge | cluster, datacenter | Effective CPU capacity available to VMs (MHz). |
| `vmware_cluster_cpu_entitled_mhz` | gauge | cluster, datacenter | CPU entitled to VMs (MHz). |
| `vmware_cluster_cpu_reserved_mhz` | gauge | cluster, datacenter | Reserved CPU (MHz). |
| `vmware_cluster_cpu_threads` | gauge | cluster, datacenter | Number of physical threads. |
| `vmware_cluster_cpu_total_mhz` | gauge | cluster, datacenter | Total cluster CPU capacity (MHz). |
| `vmware_cluster_datastores` | gauge | cluster, datacenter | Number of datastores mounted on the cluster. |
| `vmware_cluster_drs_enabled` | gauge | cluster, datacenter | 1 if DRS is enabled. |
| `vmware_cluster_drs_fully_automated` | gauge | cluster, datacenter | 1 if DRS is in fully automated mode. |
| `vmware_cluster_drs_recommendations` | gauge | cluster, datacenter | Number of pending DRS recommendations. |
| `vmware_cluster_ha_admission_control_enabled` | gauge | cluster, datacenter | 1 if HA admission control is enabled. |
| `vmware_cluster_ha_admission_control_policy` | gauge | cluster, datacenter, policy | HA admission control policy (policy label, value 1). |
| `vmware_cluster_ha_enabled` | gauge | cluster, datacenter | 1 if vSphere HA is enabled. |
| `vmware_cluster_ha_host_monitoring_enabled` | gauge | cluster, datacenter | 1 if HA host monitoring is enabled. |
| `vmware_cluster_hosts` | gauge | cluster, datacenter | Number of ESX hosts in the cluster. |
| `vmware_cluster_hosts_effective` | gauge | cluster, datacenter | Number of effective ESX hosts (connected, not in maintenance). |
| `vmware_cluster_memory_demand_bytes` | gauge | cluster, datacenter | VM memory demand. |
| `vmware_cluster_memory_effective_bytes` | gauge | cluster, datacenter | Effective memory available to VMs. |
| `vmware_cluster_memory_entitled_bytes` | gauge | cluster, datacenter | Memory entitled to VMs. |
| `vmware_cluster_memory_reserved_bytes` | gauge | cluster, datacenter | Reserved memory. |
| `vmware_cluster_memory_total_bytes` | gauge | cluster, datacenter | Total cluster memory. |
| `vmware_cluster_overall_status` | gauge | cluster, datacenter | Overall cluster status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_cluster_vmotions_total` | counter | cluster, datacenter | Number of vMotions performed in the cluster. |
| `vmware_cluster_vms` | gauge | cluster, datacenter | Total number of VMs. |
| `vmware_cluster_vms_powered_on` | gauge | cluster, datacenter | Number of powered-on VMs. |
| `vmware_cluster_vsan_enabled` | gauge | cluster, datacenter | 1 if vSAN is enabled on the cluster. |
| `vmware_cluster_dpm_enabled` (*) | gauge | datacenter, cluster | 1 if DPM (power management) is enabled. |
| `vmware_cluster_drs_score` (*) | gauge | datacenter, cluster | Cluster DRS score (0-100, VM happiness). |
| `vmware_cluster_ha_failover_cpu_resources_configured_percent` (*) | gauge | datacenter, cluster | Percentage of CPU reserved for HA failover (configured or computed). |
| `vmware_cluster_ha_failover_cpu_resources_percent` (*) | gauge | datacenter, cluster | Available HA failover CPU resources (%). |
| `vmware_cluster_ha_failover_level_configured` (*) | gauge | datacenter, cluster | Configured number of ESX host failures to tolerate ('host failures' policy). |
| `vmware_cluster_ha_failover_level_current` (*) | gauge | datacenter, cluster | Number of ESX host failures currently tolerated ('host failures' policy, slots). |
| `vmware_cluster_ha_failover_memory_resources_configured_percent` (*) | gauge | datacenter, cluster | Percentage of memory reserved for HA failover (configured or computed). |
| `vmware_cluster_ha_failover_memory_resources_percent` (*) | gauge | datacenter, cluster | Available HA failover memory resources (%). |

## ESX

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_host_boot_timestamp_seconds` | gauge | cluster, datacenter, host | Timestamp of the last ESX boot. |
| `vmware_host_certificate_expiry_timestamp_seconds` | gauge | cluster, datacenter, host | Expiry date of the ESX certificate (epoch). |
| `vmware_host_config_issues` | gauge | cluster, datacenter, host | Number of configuration issues reported on the ESX host. |
| `vmware_host_connection_state` | gauge | cluster, datacenter, host | ESX connection state to vCenter (0=disconnected, 1=connected, 2=notResponding). |
| `vmware_host_cpu_cores` | gauge | cluster, datacenter, host | Number of physical cores. |
| `vmware_host_cpu_threads` | gauge | cluster, datacenter, host | Number of physical threads. |
| `vmware_host_cpu_total_mhz` | gauge | cluster, datacenter, host | Total CPU capacity (MHz). |
| `vmware_host_cpu_usage_mhz` | gauge | cluster, datacenter, host | CPU used (MHz). |
| `vmware_host_datastores` | gauge | cluster, datacenter, host | Number of datastores mounted on the ESX host. |
| `vmware_host_hardware_status` | gauge | cluster, component, datacenter, host, name | Status of a hardware component (memory, cpu, storage) (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_info` | gauge | bios_version, build, cluster, cpu_model, datacenter, host, model, product, vendor, version | ESX host information (value always 1). |
| `vmware_host_maintenance_mode` | gauge | cluster, datacenter, host | 1 if the ESX host is in maintenance mode. |
| `vmware_host_memory_total_bytes` | gauge | cluster, datacenter, host | Total physical memory. |
| `vmware_host_memory_usage_bytes` | gauge | cluster, datacenter, host | Memory used. |
| `vmware_host_overall_status` | gauge | cluster, datacenter, host | Overall ESX status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_pnic_link_up` | gauge | cluster, datacenter, driver, host, nic, used | 1 if the physical NIC link is up. used=true if it is an uplink of a vSwitch/vDS. |
| `vmware_host_pnic_speed_mbps` | gauge | cluster, datacenter, driver, host, nic, used | Negotiated physical NIC speed (Mb/s, 0 if link is down). |
| `vmware_host_power_state` | gauge | cluster, datacenter, host | ESX power state (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_host_quarantine_mode` | gauge | cluster, datacenter, host | 1 if the ESX host is in quarantine mode (Proactive HA). |
| `vmware_host_reboot_required` | gauge | cluster, datacenter, host | 1 if an ESX reboot is required. |
| `vmware_host_sensor_health` | gauge | cluster, datacenter, host, sensor, type | Health of an IPMI/CIM hardware sensor (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_host_sensor_value` | gauge | cluster, datacenter, host, sensor, type, unit | Current value of a hardware sensor (unit in the unit label). |
| `vmware_host_service_running` | gauge | cluster, datacenter, host, policy, service | 1 if the ESX service is running (TSM-SSH, ntpd, ...). |
| `vmware_host_storage_luns` | gauge | cluster, datacenter, host | Number of LUNs seen by the ESX host. |
| `vmware_host_storage_luns_degraded` | gauge | cluster, datacenter, host | Multipath LUNs with only one live path left (redundancy lost). |
| `vmware_host_storage_luns_no_path` | gauge | cluster, datacenter, host | LUNs with no live path at all (access lost). |
| `vmware_host_storage_paths` | gauge | cluster, datacenter, host, state | Number of storage paths by state (active, standby, dead...). |
| `vmware_host_uptime_seconds` | gauge | cluster, datacenter, host | ESX host uptime. |
| `vmware_host_vms` | gauge | cluster, datacenter, host | Number of VMs registered on the ESX host. |
| `vmware_host_ha_state` (*) | gauge | datacenter, cluster, host, state | State of the ESX HA agent (FDM), value 1 for the current state. |

## Virtual machines

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_vm_connection_state` | gauge | datacenter, vm, vm_moid | VM connection state (0=connected, 1=disconnected, 2=orphaned, 3=inaccessible, 4=invalid). |
| `vmware_vm_consolidation_needed` | gauge | datacenter, vm, vm_moid | 1 if the VM disks need consolidation. |
| `vmware_vm_cpu_demand_mhz` | gauge | datacenter, vm, vm_moid | CPU demand (MHz). |
| `vmware_vm_cpu_max_mhz` | gauge | datacenter, vm, vm_moid | Maximum usable CPU (MHz). |
| `vmware_vm_cpu_usage_mhz` | gauge | datacenter, vm, vm_moid | CPU used (MHz). |
| `vmware_vm_disks` | gauge | datacenter, vm, vm_moid | Number of virtual disks. |
| `vmware_vm_guest_heartbeat_status` | gauge | datacenter, vm, vm_moid | Guest OS VMware Tools heartbeat (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vm_info` | gauge | cluster, datacenter, guest_hostname, guest_os, host, hw_version, ip_address, managed_by, template, tools_version, vm, vm_moid | VM information (value always 1). Carries the current cluster and host. |
| `vmware_vm_memory_ballooned_bytes` | gauge | datacenter, vm, vm_moid | Memory reclaimed by ballooning. |
| `vmware_vm_memory_compressed_bytes` | gauge | datacenter, vm, vm_moid | Compressed memory. |
| `vmware_vm_memory_guest_active_bytes` | gauge | datacenter, vm, vm_moid | Guest active memory. |
| `vmware_vm_memory_host_consumed_bytes` | gauge | datacenter, vm, vm_moid | Memory consumed on the ESX host. |
| `vmware_vm_memory_size_bytes` | gauge | datacenter, vm, vm_moid | Configured memory. |
| `vmware_vm_memory_swapped_bytes` | gauge | datacenter, vm, vm_moid | Memory swapped by the hypervisor. |
| `vmware_vm_nics` | gauge | datacenter, vm, vm_moid | Number of virtual NICs. |
| `vmware_vm_overall_status` | gauge | datacenter, vm, vm_moid | Overall VM status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vm_power_state` | gauge | datacenter, vm, vm_moid | VM power state (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown). |
| `vmware_vm_question_pending` | gauge | datacenter, vm, vm_moid | 1 if the VM is blocked on a question awaiting an answer. |
| `vmware_vm_snapshots` | gauge | datacenter, vm, vm_moid | Number of VM snapshots. |
| `vmware_vm_storage_committed_bytes` | gauge | datacenter, vm, vm_moid | Storage actually consumed on datastores. |
| `vmware_vm_storage_uncommitted_bytes` | gauge | datacenter, vm, vm_moid | Provisioned storage not yet consumed (thin). |
| `vmware_vm_tools_running` | gauge | datacenter, vm, vm_moid | 1 if VMware Tools is running in the guest. |
| `vmware_vm_tools_version_status` | gauge | datacenter, status, vm, vm_moid | VMware Tools version status (status label, value 1). |
| `vmware_vm_uptime_seconds` | gauge | datacenter, vm, vm_moid | VM uptime. |
| `vmware_vm_vcpus` | gauge | datacenter, vm, vm_moid | Number of vCPUs. |
| `vmware_vms_monitored` | gauge |  | Number of monitored VMs (after filters). |
| `vmware_vms_powered_on` | gauge |  | Number of monitored VMs that are powered on. |
| `vmware_vms_system_managed` | gauge |  | Number of VMs managed by a solution (vCLS, Supervisor, agents...). |
| `vmware_vms_templates` | gauge |  | Number of templates in the inventory. |
| `vmware_vm_boot_timestamp_seconds` (*) | gauge | datacenter, vm, vm_moid | Timestamp of the last VM boot. |
| `vmware_vm_guest_disk_capacity_bytes` (*) | gauge | datacenter, vm, vm_moid, disk_path | Capacity of a guest file system (via VMware Tools). |
| `vmware_vm_guest_disk_free_bytes` (*) | gauge | datacenter, vm, vm_moid, disk_path | Free space on a guest file system (via VMware Tools). |
| `vmware_vm_ha_protected` (*) | gauge | datacenter, vm, vm_moid | 1 if the VM is protected by vSphere HA. |
| `vmware_vm_snapshot_oldest_age_seconds` (*) | gauge | datacenter, vm, vm_moid | Age of the VM's oldest snapshot. |
| `vmware_vm_snapshot_oldest_timestamp_seconds` (*) | gauge | datacenter, vm, vm_moid | Creation date of the oldest snapshot. |

## Datastores

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_datastore_accessible` | gauge | datacenter, datastore, datastore_moid | 1 if the datastore is accessible. |
| `vmware_datastore_capacity_bytes` | gauge | datacenter, datastore, datastore_moid | Datastore capacity. |
| `vmware_datastore_cluster_capacity_bytes` | gauge | datacenter, datastore_cluster | Datastore cluster capacity. |
| `vmware_datastore_cluster_datastores` | gauge | datacenter, datastore_cluster | Number of datastores in the datastore cluster. |
| `vmware_datastore_cluster_free_bytes` | gauge | datacenter, datastore_cluster | Datastore cluster free space. |
| `vmware_datastore_cluster_overall_status` | gauge | datacenter, datastore_cluster | Overall datastore cluster status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_free_bytes` | gauge | datacenter, datastore, datastore_moid | Datastore free space. |
| `vmware_datastore_hosts_inaccessible` | gauge | datacenter, datastore, datastore_moid | Number of ESX hosts for which the datastore is inaccessible (APD/PDL...). |
| `vmware_datastore_hosts_mounted` | gauge | datacenter, datastore, datastore_moid | Number of ESX hosts mounting the datastore. |
| `vmware_datastore_info` | gauge | datacenter, datastore, datastore_cluster, datastore_moid, shared, type, url | Datastore information (value always 1). |
| `vmware_datastore_maintenance_mode` | gauge | datacenter, datastore, datastore_moid | 1 if the datastore is in (or entering) maintenance mode. |
| `vmware_datastore_overall_status` | gauge | datacenter, datastore, datastore_moid | Overall datastore status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_datastore_provisioned_bytes` | gauge | datacenter, datastore, datastore_moid | Total provisioned space (used + unconsumed thin). |
| `vmware_datastore_uncommitted_bytes` | gauge | datacenter, datastore, datastore_moid | Provisioned but unconsumed space (thin) on the datastore. |
| `vmware_datastore_vms` | gauge | datacenter, datastore, datastore_moid | Number of VMs with files on the datastore. |

## vSphere Distributed Switch

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_dvs_hosts` | gauge | datacenter, dvs | Number of ESX hosts that are members of the vDS. |
| `vmware_dvs_info` | gauge | datacenter, dvs, version | vDS information (value always 1). |
| `vmware_dvs_overall_status` | gauge | datacenter, dvs | Overall vDS status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_dvs_ports` | gauge | datacenter, dvs | Number of vDS ports. |
| `vmware_dvs_host_member_up` (*) | gauge | datacenter, dvs, host | 1 if the ESX member of the vDS is 'up' (otherwise outOfSync, warning, down...). |
| `vmware_dvs_host_members` (*) | gauge | datacenter, dvs, status | Number of ESX hosts that are members of the vDS, by status. |

## Networks / portgroups

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_network_accessible` | gauge | datacenter, network, network_moid, network_type | 1 if the network is accessible from at least one ESX host. |
| `vmware_network_hosts` | gauge | datacenter, network, network_moid, network_type | Number of ESX hosts with access to the network. |
| `vmware_network_overall_status` | gauge | datacenter, network, network_moid, network_type | Overall network / portgroup status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_network_vms` | gauge | datacenter, network, network_moid, network_type | Number of VMs connected to the network. |

## Resource pools

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_resourcepool_cpu_limit_mhz` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Pool CPU limit (MHz, -1 = unlimited). |
| `vmware_resourcepool_cpu_reservation_mhz` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Pool CPU reservation (MHz). |
| `vmware_resourcepool_memory_limit_bytes` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Pool memory limit (-1 = unlimited). |
| `vmware_resourcepool_memory_reservation_bytes` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Pool memory reservation. |
| `vmware_resourcepool_overall_status` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Overall resource pool status (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_resourcepool_vms` | gauge | cluster, datacenter, resource_pool, resource_pool_moid | Number of VMs directly in the resource pool. |
| `vmware_resourcepool_cpu_usage_mhz` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | CPU used by the resource pool (MHz). |
| `vmware_resourcepool_memory_ballooned_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Ballooned memory in the pool. |
| `vmware_resourcepool_memory_guest_active_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Active guest memory in the pool. |
| `vmware_resourcepool_memory_host_consumed_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Memory consumed on ESX hosts by the pool. |
| `vmware_resourcepool_memory_swapped_bytes` (*) | gauge | datacenter, cluster, resource_pool, resource_pool_moid | Swapped memory in the pool. |

## vSAN

| Metric | Type | Labels | Description |
|---|---|---|---|
| `vmware_vsan_cluster_health` (*) | gauge | datacenter, cluster | Overall vSAN health of the cluster (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_group` (*) | gauge | datacenter, cluster, group | Health of a vSAN test group (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_query_success` (*) | gauge | datacenter, cluster | 1 if the vSAN health summary could be read for the cluster. |
| `vmware_vsan_health_test` (*) | gauge | datacenter, cluster, group, test | Result of a vSAN health test (0=gray/unknown, 1=green, 2=yellow, 3=red). |
| `vmware_vsan_health_timestamp_seconds` (*) | gauge | datacenter, cluster | Timestamp of the last vSAN health test. |
