package collector

import (
	"context"
	"time"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var vmProps = []string{
	"name", "parent", "resourcePool", "overallStatus", "guestHeartbeatStatus",
	"config.managedBy", "config.version", "config.template",
	"summary.config", "summary.quickStats", "summary.storage",
	"runtime.host", "runtime.powerState", "runtime.connectionState", "runtime.bootTime",
	"runtime.consolidationNeeded", "runtime.question", "runtime.maxCpuUsage", "runtime.maxMemoryUsage",
	"runtime.dasVmProtection",
	"guest.toolsRunningStatus", "guest.toolsVersionStatus2", "guest.toolsVersion",
	"guest.guestState", "guest.ipAddress", "guest.hostName", "guest.disk", "guest.guestFullName",
	"snapshot",
}

func collectVMs(ctx context.Context, r *Run) error {
	pry := *r.Cfg.Collectors.Pryorda
	props := vmProps
	if pry {
		props = append(append([]string{}, vmProps...), pryordaVMExtraProps...)
	}
	var vms []mo.VirtualMachine
	if err := r.retrieve(ctx, []string{"VirtualMachine"}, props, &vms); err != nil {
		return err
	}
	emit := *r.Cfg.Collectors.VM
	s := r.Sink
	now := time.Now()
	var total, on, templates, system int
	for i := range vms {
		vm := &vms[i]
		// pryorda keeps templates, unlike the native series: decide first.
		var pv *pryordaVM
		if pry {
			pv = r.addPryordaVM(vm)
		}
		isTemplate := vm.Summary.Config.Template || (vm.Config != nil && vm.Config.Template)
		if isTemplate {
			templates++
			if !*r.Cfg.Filters.IncludeTemplates {
				continue
			}
		}
		managedBy := ""
		if vm.Config != nil && vm.Config.ManagedBy != nil {
			managedBy = vm.Config.ManagedBy.ExtensionKey
		}
		if managedBy != "" {
			system++
			if *r.Cfg.Filters.ExcludeSystemVMs {
				continue
			}
		}
		if r.vmExclude != nil && r.vmExclude.MatchString(vm.Name) {
			continue
		}
		if r.vmInclude != nil && !r.vmInclude.MatchString(vm.Name) {
			continue
		}
		total++
		hostName, cl, dc := "", "", ""
		if vm.Runtime.Host != nil {
			hostName = r.Topo.Name[vm.Runtime.Host.Value]
			cl = r.Topo.ClusterOfHost(vm.Runtime.Host.Value)
			dc = r.Topo.Datacenter(vm.Runtime.Host.Value)
		}
		if dc == "" && vm.ResourcePool != nil {
			dc = r.Topo.Datacenter(vm.ResourcePool.Value)
			cl = r.Topo.ClusterOfPool(vm.ResourcePool.Value)
		}
		// Stable labels: host/cluster change on every vMotion and would break
		// series continuity and alert "for:" durations. They are carried only
		// by vmware_vm_info (join on(vcenter, vm_moid)).
		l := L{"dc_name", dc, "vm_name", vm.Name, "vm_moid", vm.Self.Value, "folder_name", r.Topo.FolderOf(vm.Parent)}
		powered := vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn
		if powered {
			on++
			r.addPerfVM(perfEntity{ref: vm.Self, labels: l, pryVM: pv})
		}
		if !emit {
			continue
		}

		sc := vm.Summary.Config
		hw := ""
		if vm.Config != nil {
			hw = vm.Config.Version
		}
		guestOS := sc.GuestFullName
		if vm.Guest != nil && vm.Guest.GuestFullName != "" {
			guestOS = vm.Guest.GuestFullName
		}
		ip, gh, toolsVer := "", "", ""
		if vm.Guest != nil {
			ip, gh, toolsVer = vm.Guest.IpAddress, vm.Guest.HostName, vm.Guest.ToolsVersion
		}
		s.Gauge(n("vm_info"), "VM information (value always 1). Carries the current cluster and host.", 1, append(append(L{}, l...),
			"cluster_name", cl, "host_name", hostName, "guest_os", guestOS, "hw_version", hw, "tools_version", toolsVer,
			"ip_address", ip, "guest_hostname", gh, "managed_by", managedBy, "template", boolStr(isTemplate)))

		s.Gauge(n("vm_power_status"), "VM power state"+powerHelp+".", vmPower(vm.Runtime.PowerState), l)
		s.Gauge(n("vm_connection_state"), "VM connection state"+vmConnHelp+".", vmConn(vm.Runtime.ConnectionState), l)
		s.Gauge(n("vm_overall_status"), "Overall status of the VM"+statusHelp+".", statusValue(vm.OverallStatus), l)
		s.Gauge(n("vm_guest_heartbeat_status"), "VMware Tools heartbeat of the guest OS"+statusHelp+".", statusValue(vm.GuestHeartbeatStatus), l)
		s.Gauge(n("vm_consolidation_needed"), "1 if the VM disks need consolidation.", b2f(vm.Runtime.ConsolidationNeeded), l)
		s.Gauge(n("vm_question_pending"), "1 if the VM is blocked on a pending question.", b2f(vm.Runtime.Question != nil), l)
		if vm.Runtime.DasVmProtection != nil {
			s.Gauge(n("vm_ha_protected"), "1 if the VM is protected by vSphere HA.", b2f(vm.Runtime.DasVmProtection.DasProtected), l)
		}
		if vm.Runtime.BootTime != nil {
			s.Gauge(n("vm_boot_time_seconds"), "Time of the last VM boot.", float64(vm.Runtime.BootTime.Unix()), l)
		}

		s.Gauge(n("vm_vcpus"), "Number of vCPUs.", float64(sc.NumCpu), l)
		s.Gauge(n("vm_memory_size_bytes"), "Configured memory.", float64(sc.MemorySizeMB)*1024*1024, l)
		s.Gauge(n("vm_disks"), "Number of virtual disks.", float64(sc.NumVirtualDisks), l)
		s.Gauge(n("vm_nics"), "Number of virtual NICs.", float64(sc.NumEthernetCards), l)
		s.Gauge(n("vm_cpu_max_mhz"), "Maximum usable CPU (MHz).", float64(vm.Runtime.MaxCpuUsage), l)

		qs := vm.Summary.QuickStats
		s.Gauge(n("vm_cpu_usage_mhz"), "CPU usage (MHz).", float64(qs.OverallCpuUsage), l)
		s.Gauge(n("vm_cpu_demand_mhz"), "CPU demand (MHz).", float64(qs.OverallCpuDemand), l)
		s.Gauge(n("vm_memory_guest_active_bytes"), "Active guest memory.", float64(qs.GuestMemoryUsage)*1024*1024, l)
		s.Gauge(n("vm_memory_host_consumed_bytes"), "Host memory consumed.", float64(qs.HostMemoryUsage)*1024*1024, l)
		s.Gauge(n("vm_memory_ballooned_bytes"), "Memory reclaimed by ballooning.", float64(qs.BalloonedMemory)*1024*1024, l)
		s.Gauge(n("vm_memory_swapped_bytes"), "Memory swapped by the hypervisor.", float64(qs.SwappedMemory)*1024*1024, l)
		s.Gauge(n("vm_memory_compressed_bytes"), "Compressed memory.", float64(qs.CompressedMemory)*1024, l)
		s.Gauge(n("vm_uptime_seconds"), "VM uptime.", float64(qs.UptimeSeconds), l)

		if st := vm.Summary.Storage; st != nil {
			s.Gauge(n("vm_storage_committed_bytes"), "Storage actually consumed on datastores.", float64(st.Committed), l)
			s.Gauge(n("vm_storage_uncommitted_bytes"), "Provisioned storage not yet consumed (thin).", float64(st.Uncommitted), l)
		}

		if vm.Guest != nil {
			s.Gauge(n("vm_tools_running"), "1 if VMware Tools is running in the guest.", b2f(vm.Guest.ToolsRunningStatus == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning)), l)
			ts := vm.Guest.ToolsVersionStatus2
			if ts == "" {
				ts = "unknown"
			}
			s.Gauge(n("vm_tools_version_status"), "VMware Tools version status (status label, value 1).", 1, append(append(L{}, l...), "status", ts))
			for _, d := range vm.Guest.Disk {
				dl := append(append(L{}, l...), "disk_path", d.DiskPath)
				s.Gauge(n("vm_guest_disk_capacity_bytes"), "Capacity of a guest filesystem (via VMware Tools).", float64(d.Capacity), dl)
				s.Gauge(n("vm_guest_disk_free_bytes"), "Free space of a guest filesystem (via VMware Tools).", float64(d.FreeSpace), dl)
			}
		}

		count, oldest := 0, time.Time{}
		if vm.Snapshot != nil {
			walkSnapshots(vm.Snapshot.RootSnapshotList, func(t time.Time) {
				count++
				if oldest.IsZero() || t.Before(oldest) {
					oldest = t
				}
			})
		}
		s.Gauge(n("vm_snapshots_existing"), "Number of VM snapshots.", float64(count), l)
		if !oldest.IsZero() {
			s.Gauge(n("vm_snapshot_oldest_age_seconds"), "Age of the oldest VM snapshot.", now.Sub(oldest).Seconds(), l)
			s.Gauge(n("vm_snapshot_oldest_timestamp_seconds"), "Creation time of the oldest snapshot.", float64(oldest.Unix()), l)
		}
	}
	if emit {
		s.Gauge(n("vms_monitored"), "Number of monitored VMs (after filters).", float64(total), nil)
		s.Gauge(n("vms_powered_on"), "Number of monitored powered-on VMs.", float64(on), nil)
		s.Gauge(n("vms_templates"), "Number of templates in the inventory.", float64(templates), nil)
		s.Gauge(n("vms_system_managed"), "Number of VMs managed by a solution (vCLS, Supervisor, agents...).", float64(system), nil)
	}
	return nil
}

func walkSnapshots(list []types.VirtualMachineSnapshotTree, fn func(time.Time)) {
	for _, sn := range list {
		fn(sn.CreateTime)
		walkSnapshots(sn.ChildSnapshotList, fn)
	}
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
