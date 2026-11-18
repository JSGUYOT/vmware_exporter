# Alerting rules explained

What each shipped alert is for, why it fires, and whether vCenter shows the same thing. Use it when
Alertmanager reports a problem that you cannot find in the vSphere Client.

Rules: [vmware-alerts.yml](../deploy/prometheus/vmware-alerts.yml) (78 alerts) and
[pryorda-alerts.yml](../deploy/prometheus/pryorda-alerts.yml) (5 alerts). The `*-supervised.yml`
variants have the same alerts, filtered on a VM tag (see the [README](../README.md#alerting-only-on-the-vms-tagged-for-supervision)).

## Why an alert can be missing from vCenter

The vSphere Client only raises a red or yellow indicator when an **alarm definition** matches. Most
metrics have none: the value is on a page, but nothing flags it. The usual reasons, most common first:

1. **No alarm is defined on that condition.** The value is visible if you open the right page, but
   nothing draws attention to it. CPU ready, snapshot age and guest disk space are examples.
2. **The thresholds differ.** vCenter and the rule can look at the same value with a different limit
   or duration, so one fires and the other does not.
3. **The rule measures something vCenter does not compute**: a projection (datastore full in 7 days),
   a ratio (packet loss), or a VM excluded from a total (dedicated CPUs).
4. **The information is outside the inventory**: in the appliance interface (VAMI, port 5480) or in
   Skyline Health, whose findings do not all appear as triggered alarms.
5. **The alarm was acknowledged, reset or disabled**, or an event-based alarm was cleared by hand
   while the problem is still there. The rules read the current state, not the alarm.
6. **It is a problem of the monitoring itself**, such as the exporter failing to read vCenter.

## Reading the "In vCenter" column

| Value | Meaning |
|---|---|
| **Alarm** | A default vCenter alarm covers this condition. It should appear in *Monitor > Issues and Alarms > Triggered Alarms*, possibly with other thresholds. |
| **Shown** | The value is visible on the page given, but no default alarm covers it. No indicator is expected. |
| **Elsewhere** | Visible in VAMI or Skyline Health, not as an inventory alarm. |
| **No** | vCenter does not have this information. |

`(?)` means the entry should be confirmed on your vCenter.

**How reliable this column is.** Broadcom publishes no complete list that could be checked here, and
the default alarms change between versions (some were renamed or removed in 8.x). Only these alarm
names were confirmed in Broadcom documentation: *Host connection and power state*, *ESXi Host
Certificate Status*, *Virtual machine CPU usage*, *Virtual machine memory usage* and *vSphere HA host
status*. The other names and all the vCenter thresholds come from general knowledge of vSphere and
were not checked against a live vCenter. The reference for your environment is
*vCenter > Configure > More > Alarm Definitions*.

To see the alarms vCenter has active right now, as read by the exporter:
`vmware_alarm_triggered`. If an alert has no matching series there, vCenter has no active alarm for it.

## Exporter

About the monitoring itself. vCenter knows nothing about them.

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareExporterTargetDown` | critical | `vmware_up` is 0 for 5 min | The exporter cannot connect or log in to vCenter. Check vCenter, the network and the service account. | **No** |
| `VMwareExporterScrapeFailed` | critical | Prometheus `up` is 0 for 5 min | Prometheus cannot read `/metrics`. All other alerts stop being evaluated on fresh data. | **No** |
| `VMwareExporterStaleData` | warning | Last collection older than 10 min, for 5 min | Collection is stuck: timeout too short or vCenter very slow. | **No** |
| `VMwareExporterCollectorFailing` | warning | A collector fails for 15 min | Part of the data is missing: permissions, missing endpoint, timeout. See the exporter logs. | **No** |
| `VMwareExporterSlowCollection` | info | Collection uses over 80% of its timeout for 15 min | It may soon be cut. Increase `interval`/`timeout` or reduce the scope. | **No** |
| `VMwareExporterTagsReadFailing` | critical | Tags cannot be read for 15 min | Tag changes are ignored. If tags were never read, rules filtered on a tag stay silent. | **No** |

## vCenter and appliance

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareVCenterAlarmCritical` | critical | A red, unacknowledged alarm is active for 2 min | Relays any native vCenter alarm, including those with no dedicated rule here. | **Alarm**: this is the alarm itself. |
| `VMwareVCenterAlarmWarning` | warning | A yellow, unacknowledged alarm is active for 10 min | Same, for yellow alarms. Acknowledging the alarm in the vSphere Client clears the alert. | **Alarm**: this is the alarm itself. |
| `VMwareVCenterCertificateExpiringSoon` | warning | Certificate served by vCenter expires within 30 days, for 1 h | Renew the machine SSL certificate. | **Alarm**: *Certificate Status* (?). Shown in *Administration > Certificates*. Its limit may differ. |
| `VMwareVCenterCertificateExpiryCritical` | critical | Same, within 7 days, for 10 min | Expiry is close: clients and services will fail. | **Alarm**: same. |
| `VMwareApplianceHealthDegraded` | warning | A VAMI health component is yellow or worse for 10 min | CPU, memory, storage, database or similar under strain. | **Elsewhere**: VAMI *Summary*. Some components also have alarms (?). |
| `VMwareApplianceHealthCritical` | critical | A VAMI health component is red for 5 min | The appliance itself is in trouble. | **Elsewhere**: VAMI *Summary*. |
| `VMwareApplianceServiceStopped` | critical | An automatic-start service is stopped for 10 min | A vCenter service is down although it should run. | **Elsewhere**: VAMI *Services*. |
| `VMwareApplianceServiceDegraded` | warning | A service reports DEGRADED for 15 min | The service runs but is not healthy. | **Elsewhere**: VAMI *Services*. |
| `VMwareVCenterBackupFailed` | warning | The last file-based backup failed, for 30 min | Your restore point is getting old. | **Elsewhere**: VAMI *Backup*. A default alarm may exist (?). |
| `VMwareVCenterBackupTooOld` | warning | Last successful backup older than 26 h, for 1 h | The schedule did not run or keeps failing. | **Elsewhere**: VAMI *Backup*. No alarm on the age. |
| `VMwareVCenterNoSuccessfulBackup` | warning | Backup jobs are known, none succeeded, for 6 h | Backups are not configured or never worked. | **Elsewhere**: VAMI *Backup*. |

## Clusters

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareClusterStatusRed` | critical | Overall status is red for 5 min | Something on the cluster is in error. | **Alarm**: the status comes from the alarms on the object. |
| `VMwareClusterHADisabled` | warning | HA is off with more than 1 host, for 30 min | VMs are not restarted if a host fails. | **Shown**: *Cluster > Configure > vSphere Availability*. It is a setting, not a fault. |
| `VMwareClusterHAFailoverCapacityLost` | critical | Failover capacity is below the configured reserve for 15 min | The cluster can no longer guarantee VM restarts. | **Alarm**: *Insufficient vSphere HA failover resources*. |
| `VMwareClusterHostsUnavailable` | warning | Hosts minus effective hosts is above 0 for 15 min | Hosts are disconnected, not responding or in maintenance. | **Shown**: *Cluster > Hosts*. Maintenance mode raises no alarm. |
| `VMwareClusterMemoryPressure` | warning | Memory demand is above 90% of effective capacity for 30 min | The cluster is short of memory for its VMs. | **Shown**: *Cluster > Monitor > Resource Allocation*. No alarm at cluster level. |
| `VMwareClusterCpuPressure` | warning | CPU demand is above 85% of effective capacity for 30 min, [dedicated CPUs](#vms-with-dedicated-cpus) excluded | The cluster is short of CPU. | **Shown**: same. vCenter's figures include dedicated VMs. |
| `VMwareClusterDrsScoreLow` | warning | DRS is on and the score is below 60 for 1 h | VMs are not getting the resources they demand. | **Shown**: *Cluster > Summary* and *Monitor > vSphere DRS*. No alarm on the score. |
| `VMwareClusterConfigIssues` | info | Configuration issues exist for 1 h | vCenter reports a problem in the cluster's configuration. | **Shown**: banner on *Cluster > Summary*. |

## ESX hosts

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareHostNotResponding` | critical | Connection state is notResponding for 3 min | vCenter receives no heartbeat from the host. | **Alarm**: *Host connection and power state*. |
| `VMwareHostDisconnected` | warning | Disconnected and not in maintenance, for 10 min | The host is out of vCenter's management. | **Shown**: in the inventory. A manual disconnect may raise no alarm (?). |
| `VMwareHostStatusRed` | critical | Overall status is red for 5 min | An alarm or hardware problem on the host. | **Alarm**: the status comes from the alarms on the object. |
| `VMwareHostMaintenanceMode` | info | In maintenance for over 24 h | A reminder to exit maintenance mode. | **Shown**: in the inventory. Maintenance is a normal state. |
| `VMwareHostQuarantine` | warning | In quarantine mode for 5 min | Proactive HA: the hardware vendor reported a partial failure. | **Shown**: on the host's *Summary* (?). |
| `VMwareHostHAAgentProblem` | warning | HA agent unhealthy, outside maintenance, for 10 min | Isolation, network partition or initialization error. | **Alarm**: *vSphere HA host status*. |
| `VMwareHostSensorCritical` | critical | A hardware sensor is red for 5 min | Temperature, fan, power, and so on. | **Alarm**: *Host hardware ... status* family. Shown in *Host > Monitor > Hardware Health*. |
| `VMwareHostSensorWarning` | warning | A hardware sensor is yellow for 30 min | Same, less urgent. | **Alarm**: same. |
| `VMwareHostHardwareComponentFailed` | critical | A hardware component is in error for 5 min | CPU, memory or storage component failed. | **Alarm**: same family. |
| `VMwareHostStorageLunNoPath` | critical | LUNs with no live path for 3 min | Storage access lost (APD/PDL). | **Alarm**: *Cannot connect to storage*, event-based: it can be cleared while the fault remains. |
| `VMwareHostStoragePathRedundancyLost` | warning | LUNs with a single live path for 10 min | Still working, but the next failure cuts access. | **Alarm**: same (?). Shown in *Host > Configure > Storage Devices > Paths*. |
| `VMwareHostStoragePathDead` | warning | Dead paths for 10 min | Check zoning and links. | **Shown**: same page. An alarm only if redundancy is lost (?). |
| `VMwareHostUplinkDown` | warning | A used physical NIC has no link for 5 min, host connected | A cable, port or NIC is down. | **Shown**: *Host > Configure > Physical adapters*. Alarm *Network uplink redundancy lost* only if redundancy is lost (?). |
| `VMwareHostCertificateExpiringSoon` | warning | Host certificate expires within 30 days, for 1 h | Renew the certificate. | **Alarm**: *ESXi Host Certificate Status*, renamed in 8.0 U3h. Its limit may differ. |
| `VMwareHostSSHEnabled` | info | SSH service running for over 4 h | Hardening: stop SSH outside maintenance windows. | **Shown**: *Host > Configure > Services*, and usually a banner on *Summary*. No alarm. |
| `VMwareHostNTPStopped` | warning | ntpd or chronyd stopped although enabled, for 30 min | Clock drift can break HA, vSAN, authentication and certificates. | **Shown**: *Host > Configure > Services*. No alarm (?). |
| `VMwareHostRebootRequired` | info | Reboot pending for over 24 h | A patch or change waits for a reboot. | **Shown**: on the host's *Summary*. No alarm. |
| `VMwareHostCpuHigh` | warning | CPU above 90% for 30 min, [dedicated CPUs](#vms-with-dedicated-cpus) excluded | Sustained host load. | **Alarm**: *Host CPU usage*, with lower, shorter thresholds, so vCenter usually fires first. |
| `VMwareHostMemoryHigh` | warning | Memory above 95% for 30 min | Risk of ballooning and swapping. A high value is often normal on ESXi. | **Alarm**: *Host memory usage*, with lower thresholds. |
| `VMwareHostDatastoreLatencyHigh` | warning | Average latency to a datastore above 30 ms for 15 min | Storage is slow or saturated. | **Shown**: *Host > Monitor > Performance > Advanced*. No alarm. |
| `VMwareHostNetworkDrops` | warning | Over 0.5% of packets dropped for 15 min | Network saturation, or guests not emptying their receive buffers. | **Shown**: same page. No alarm. vCenter does not compute this ratio. |
| `VMwareHostNetworkErrors` | warning | Over 10 errors per 20 s for 15 min | Check cable, SFP and switch port. | **Shown**: same page. No alarm. |

## VMs

This is where most alerts have no vCenter counterpart: **no default alarm exists on these values**,
unless stated otherwise.

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareVMHeartbeatLost` | critical | Powered on, Tools heartbeat red for 5 min | The guest OS may be hung. | **Shown**: *VM > Summary*. An HA alarm exists only if VM Monitoring is on. |
| `VMwareVMToolsNotRunning` | warning | Powered on for over 15 min, Tools stopped for 30 min | No clean shutdown, guest monitoring or quiescing. | **Shown**: *VM > Summary*. |
| `VMwareVMToolsOutdated` | info | Tools version flagged for 24 h | Update VMware Tools. | **Shown**: *VM > Summary*. |
| `VMwareVMQuestionPending` | critical | A question is pending for 2 min | The VM is blocked until someone answers. | **Shown**: *VM > Summary*, with the question to answer. |
| `VMwareVMConsolidationNeeded` | warning | Consolidation needed for 1 h | Delta files were left behind, often by a backup. | **Alarm**: *Virtual machine Consolidation Needed status*. |
| `VMwareVMSnapshotTooOld` | warning | Oldest snapshot older than 3 days, for 1 h | Snapshots grow and are not backups. | **Shown**: *VM > Snapshots*. |
| `VMwareVMTooManySnapshots` | warning | More than 3 snapshots for 1 h | Long chains degrade disk performance. | **Shown**: *VM > Snapshots*. |
| `VMwareVMGuestDiskFull` | critical | Guest filesystem below 5% free for 10 min | The OS or application may stop. Needs VMware Tools. | **Shown**: *VM > Summary* (?). vCenter alarms watch datastores, not guest disks. |
| `VMwareVMGuestDiskLow` | warning | Guest filesystem below 10% free for 30 min | Plan for more space. | **Shown**: same. |
| `VMwareVMCpuReadyHigh` | warning | CPU ready above 10% per vCPU for 15 min | The VM waits for a physical CPU: contention or too many vCPUs. | **Shown**: *VM > Monitor > Performance > Advanced*, chart *Ready*. |
| `VMwareVMCoStopHigh` | warning | Co-stop above 3% per vCPU for 15 min | Too many vCPUs for the workload. | **Shown**: same page, chart *Co-stop*. |
| `VMwareVMMemorySwapping` | warning | Swap-in above 0 for 15 min | The VM is reading memory back from disk, so it is slow right now. See [swap](#swap-and-ballooning). | **Shown**: same page, *Swap in rate*. |
| `VMwareVMMemoryBallooned` | info | Ballooned memory above 0 for 30 min | ESXi reclaims memory from the guest: contention or a limit. See [swap](#swap-and-ballooning). | **Shown**: *VM > Monitor > Utilization*. |
| `VMwareVMDiskLatencyHigh` | warning | Maximum disk latency above 50 ms for 15 min | Slow storage seen by this VM. | **Shown**: *VM > Monitor > Performance > Advanced*. |
| `VMwareVMNotHAProtected` | warning | Powered on in an HA cluster, not protected, for 30 min | The VM would not restart if its host fails. | **Shown**: *VM > Summary*, vSphere HA protection. |
| `VMwareVMConnectionProblem` | warning | Connection state is not "connected" for 10 min | Disconnected, orphaned, inaccessible or invalid. | **Shown**: greyed out in the inventory. |

vCenter's *Virtual machine CPU usage* and *Virtual machine memory usage* alarms have no native
counterpart here. They reach Prometheus through the two relay alerts above.

## Datastores

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareDatastoreAlmostFull` | critical | Above 95% used for 5 min | VMs may stop (thin disks, snapshots). | **Alarm**: *Datastore usage on disk*, with lower thresholds, so vCenter usually fires first. |
| `VMwareDatastoreUsageHigh` | warning | Above 85% used for 30 min | Free space or extend the datastore. | **Alarm**: same. |
| `VMwareDatastoreFullIn7Days` | warning | Linear projection of the last 24 h is below 0 in 7 days, for 1 h | It will fill up soon, even if usage still looks fine. | **No**: vCenter makes no projection. |
| `VMwareDatastoreOverprovisioned` | info | Provisioned above 200% of capacity for 6 h | Thin disks could fill it if they all grow. | **Shown**: *Datastore > Summary*. No alarm. |
| `VMwareDatastoreInaccessible` | critical | Not accessible for 2 min | vCenter has lost the datastore. | **Shown**: flagged in the inventory. Possibly *Cannot connect to storage* (?). |
| `VMwareDatastoreInaccessibleFromHosts` | critical | Inaccessible from some hosts for 5 min | APD/PDL or storage network problem on those hosts. | **Shown**: *Datastore > Hosts*. Possibly the same alarm (?). |
| `VMwareDatastoreStatusRed` | critical | Overall status is red for 5 min | Usually a triggered usage or connectivity alarm. | **Alarm**: the status comes from the alarms on the object. |

## Network

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareDVSStatusRed` | critical | vDS status is red for 5 min | Something is in error on the distributed switch. | **Alarm**: the status comes from the alarms on the object. |
| `VMwareDVSHostOutOfSync` | warning | A host is out of sync or down on the vDS for 10 min | The host's configuration differs from vCenter's. | **Shown**: *vDS > Configure > Hosts*. No default alarm (?). |
| `VMwareNetworkInaccessible` | warning | Portgroup with hosts, reachable from none, for 10 min | VMs on it have no network. | **Shown**: *Network > Summary*. |

## vSAN

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMwareVSANHealthRed` | critical | Cluster health is red for 5 min | At least one vSAN health test failed. | **Alarm**: vSAN health alarms, some removed in 8.x. **Elsewhere**: *Cluster > Monitor > vSAN > Skyline Health*. |
| `VMwareVSANHealthYellow` | warning | Cluster health is yellow for 1 h | At least one health test is in warning. | Same. |
| `VMwareVSANTestFailed` | warning | A given health test is red for 15 min | Names the failing test and its group. | **Elsewhere**: Skyline Health. |
| `VMwareVSANHealthUnavailable` | warning | Health query fails for 30 min | The health service is down, or the account lacks rights. | **No**: it is the exporter's read that fails. |
| `VMwareVSANHealthStale` | info | Last health check older than 3 h, for 30 min | The health checks are not running on schedule. | **Elsewhere**: the check time is in Skyline Health. No alarm. |

## pryorda rules

For teams migrating from pryorda/vmware_exporter. They need `collectors: {pryorda: true}`.

| Alert | Severity | Fires when | Meaning | In vCenter |
|---|---|---|---|---|
| `VMWarnMemoryUsage` | warning | VM memory between 90% and 95% for 30 min | High memory use inside the VM's allocation. | **Alarm**: *Virtual machine memory usage*, with other thresholds. |
| `VMCritMemoryUsage` | critical | VM memory at or above 95% for 5 min | Same, very high. | **Alarm**: same. |
| `VMWarnNumberSnapshots` | warning | Fewer than 3 snapshots, for 30 min | The upstream rule has no lower bound: a VM with 1 or 2 snapshots fires. See [below](#pryorda-snapshot-count). | **Shown**: *VM > Snapshots*. |
| `VMCritNumberSnapshots` | critical | 3 or more snapshots, for 30 min | Long snapshot chain. | **Shown**: same. |
| `VMWarnAgeSnapshots` | warning | A snapshot is at least 7 days old, for 5 min | Old snapshot. | **Shown**: same. |

## Details on some alerts

### Swap and ballooning

`vmware_vm_memory_swapped_bytes` is the amount of VM memory still in the `.vswp` file. Pages stay there
until the guest reads them again, so this value can remain above zero for weeks after a contention has
ended, with no current impact. That is why the alert watches the **swap-in rate**, which is only above
zero when the VM is being slowed down now.

| Swapped | Swap-in | Reading |
|---|---|---|
| above 0 | 0 | Leftover from past contention. No alert. |
| above 0 | above 0 | The VM is penalized now. `VMwareVMMemorySwapping` fires after 15 min. |

Common causes: a memory limit on the VM or its resource pool, or memory contention on the host.
A full power off and on (not a guest reboot) recreates the swap file and resets the value.

### VMs with dedicated CPUs

A powered-on VM with latency sensitivity `high` and a full CPU reservation has its cores for itself.
ESXi can show them as busy whatever the guest does. `VMwareHostCpuHigh` and `VMwareClusterCpuPressure`
remove these VMs from both the load and the capacity, so a figure can differ from what the vSphere
Client shows for the same host. See [Latency-sensitive VMs](../README.md#latency-sensitive-vms).

### pryorda snapshot count

`VMWarnNumberSnapshots` is `vmware_vm_snapshots < 3`, kept unchanged from upstream. The exporter emits
this series only for VMs that have a snapshot tree, so a VM with no snapshot normally does not fire,
but one with a single snapshot does. It reports the presence of a snapshot rather than a high number.

### Thresholds

All thresholds are starting values, to be tuned to your environment in the rules file. If you edit
`vmware-alerts.yml`, regenerate the filtered file:
`cd deploy/prometheus && python3 build_supervised_alerts.py`.
