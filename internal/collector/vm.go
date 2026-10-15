package collector

import (
	"context"
	"time"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var vmProps = []string{
	"name", "resourcePool", "overallStatus", "guestHeartbeatStatus",
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
	var vms []mo.VirtualMachine
	if err := r.retrieve(ctx, []string{"VirtualMachine"}, vmProps, &vms); err != nil {
		return err
	}
	emit := *r.Cfg.Collectors.VM
	s := r.Sink
	now := time.Now()
	var total, on, templates, system int
	for i := range vms {
		vm := &vms[i]
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
		// Labels stables : host/cluster changent à chaque vMotion et casseraient la
		// continuité des séries et les délais "for:" des alertes. Ils sont portés
		// uniquement par vmware_vm_info (jointure on(vcenter, vm_moid)).
		l := L{"datacenter", dc, "vm", vm.Name, "vm_moid", vm.Self.Value}
		powered := vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn
		if powered {
			on++
			r.addPerfVM(perfEntity{ref: vm.Self, labels: l})
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
		s.Gauge(n("vm_info"), "Informations sur la VM (valeur toujours 1). Porte cluster et host courants.", 1, append(append(L{}, l...),
			"cluster", cl, "host", hostName, "guest_os", guestOS, "hw_version", hw, "tools_version", toolsVer,
			"ip_address", ip, "guest_hostname", gh, "managed_by", managedBy, "template", boolStr(isTemplate)))

		s.Gauge(n("vm_power_state"), "État d'alimentation de la VM"+powerHelp+".", vmPower(vm.Runtime.PowerState), l)
		s.Gauge(n("vm_connection_state"), "État de connexion de la VM"+vmConnHelp+".", vmConn(vm.Runtime.ConnectionState), l)
		s.Gauge(n("vm_overall_status"), "Statut global de la VM"+statusHelp+".", statusValue(vm.OverallStatus), l)
		s.Gauge(n("vm_guest_heartbeat_status"), "Heartbeat VMware Tools du système invité"+statusHelp+".", statusValue(vm.GuestHeartbeatStatus), l)
		s.Gauge(n("vm_consolidation_needed"), "1 si les disques de la VM doivent être consolidés.", b2f(vm.Runtime.ConsolidationNeeded), l)
		s.Gauge(n("vm_question_pending"), "1 si la VM est bloquée sur une question en attente de réponse.", b2f(vm.Runtime.Question != nil), l)
		if vm.Runtime.DasVmProtection != nil {
			s.Gauge(n("vm_ha_protected"), "1 si la VM est protégée par vSphere HA.", b2f(vm.Runtime.DasVmProtection.DasProtected), l)
		}
		if vm.Runtime.BootTime != nil {
			s.Gauge(n("vm_boot_timestamp_seconds"), "Horodatage du dernier démarrage de la VM.", float64(vm.Runtime.BootTime.Unix()), l)
		}

		s.Gauge(n("vm_vcpus"), "Nombre de vCPU.", float64(sc.NumCpu), l)
		s.Gauge(n("vm_memory_size_bytes"), "Mémoire configurée.", float64(sc.MemorySizeMB)*1024*1024, l)
		s.Gauge(n("vm_disks"), "Nombre de disques virtuels.", float64(sc.NumVirtualDisks), l)
		s.Gauge(n("vm_nics"), "Nombre de cartes réseau virtuelles.", float64(sc.NumEthernetCards), l)
		s.Gauge(n("vm_cpu_max_mhz"), "CPU maximum utilisable (MHz).", float64(vm.Runtime.MaxCpuUsage), l)

		qs := vm.Summary.QuickStats
		s.Gauge(n("vm_cpu_usage_mhz"), "CPU consommé (MHz).", float64(qs.OverallCpuUsage), l)
		s.Gauge(n("vm_cpu_demand_mhz"), "Demande CPU (MHz).", float64(qs.OverallCpuDemand), l)
		s.Gauge(n("vm_memory_guest_active_bytes"), "Mémoire active de l'invité.", float64(qs.GuestMemoryUsage)*1024*1024, l)
		s.Gauge(n("vm_memory_host_consumed_bytes"), "Mémoire consommée sur l'ESX.", float64(qs.HostMemoryUsage)*1024*1024, l)
		s.Gauge(n("vm_memory_ballooned_bytes"), "Mémoire récupérée par ballooning.", float64(qs.BalloonedMemory)*1024*1024, l)
		s.Gauge(n("vm_memory_swapped_bytes"), "Mémoire swappée par l'hyperviseur.", float64(qs.SwappedMemory)*1024*1024, l)
		s.Gauge(n("vm_memory_compressed_bytes"), "Mémoire compressée.", float64(qs.CompressedMemory)*1024, l)
		s.Gauge(n("vm_uptime_seconds"), "Uptime de la VM.", float64(qs.UptimeSeconds), l)

		if st := vm.Summary.Storage; st != nil {
			s.Gauge(n("vm_storage_committed_bytes"), "Stockage réellement consommé sur les datastores.", float64(st.Committed), l)
			s.Gauge(n("vm_storage_uncommitted_bytes"), "Stockage provisionné non encore consommé (thin).", float64(st.Uncommitted), l)
		}

		if vm.Guest != nil {
			s.Gauge(n("vm_tools_running"), "1 si VMware Tools tourne dans l'invité.", b2f(vm.Guest.ToolsRunningStatus == string(types.VirtualMachineToolsRunningStatusGuestToolsRunning)), l)
			ts := vm.Guest.ToolsVersionStatus2
			if ts == "" {
				ts = "unknown"
			}
			s.Gauge(n("vm_tools_version_status"), "Statut de version des VMware Tools (label status, valeur 1).", 1, append(append(L{}, l...), "status", ts))
			for _, d := range vm.Guest.Disk {
				dl := append(append(L{}, l...), "disk_path", d.DiskPath)
				s.Gauge(n("vm_guest_disk_capacity_bytes"), "Capacité d'un système de fichiers de l'invité (via VMware Tools).", float64(d.Capacity), dl)
				s.Gauge(n("vm_guest_disk_free_bytes"), "Espace libre d'un système de fichiers de l'invité (via VMware Tools).", float64(d.FreeSpace), dl)
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
		s.Gauge(n("vm_snapshots"), "Nombre de snapshots de la VM.", float64(count), l)
		if !oldest.IsZero() {
			s.Gauge(n("vm_snapshot_oldest_age_seconds"), "Âge du plus ancien snapshot de la VM.", now.Sub(oldest).Seconds(), l)
			s.Gauge(n("vm_snapshot_oldest_timestamp_seconds"), "Date de création du plus ancien snapshot.", float64(oldest.Unix()), l)
		}
	}
	if emit {
		s.Gauge(n("vms_monitored"), "Nombre de VMs supervisées (après filtres).", float64(total), nil)
		s.Gauge(n("vms_powered_on"), "Nombre de VMs supervisées allumées.", float64(on), nil)
		s.Gauge(n("vms_templates"), "Nombre de templates dans l'inventaire.", float64(templates), nil)
		s.Gauge(n("vms_system_managed"), "Nombre de VMs gérées par une solution (vCLS, Supervisor, agents...).", float64(system), nil)
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
