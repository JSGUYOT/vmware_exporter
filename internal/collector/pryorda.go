package collector

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Compatibility with pryorda/vmware_exporter (0.18.x): same metric names
// (typos included), label names, units and values, so dashboards and alerts
// built for it keep working. Help texts are copied verbatim. Differences:
// every series also carries the "vcenter" label (and extra_labels), and the
// optional tags / custom attributes / alarms of pryorda are not implemented.

// Real-time counters queried by pryorda (raw vSphere units, latest sample).
var (
	pryordaVMCounters = []string{
		"cpu.ready.summation", "cpu.maxlimited.summation", "cpu.usage.average", "cpu.usagemhz.average",
		"cpu.costop.summation", "cpu.idle.summation", "cpu.demand.average",
		"mem.usage.average", "mem.consumed.average", "mem.active.average", "mem.swapped.average", "mem.vmmemctl.average",
		"disk.maxTotalLatency.latest", "disk.usage.average", "disk.read.average", "disk.write.average",
		"net.received.average", "net.transmitted.average", "net.multicastRx.summation", "net.multicastTx.summation",
		"net.broadcastTx.summation", "net.broadcastRx.summation", "net.droppedRx.summation", "net.droppedTx.summation",
	}
	pryordaHostCounters = []string{
		"cpu.costop.summation", "cpu.demand.average", "cpu.idle.summation", "cpu.ready.summation",
		"cpu.swapwait.summation", "cpu.usage.average", "cpu.usagemhz.average", "cpu.used.summation",
		"disk.read.average", "disk.write.average",
		"mem.active.average", "mem.latency.average", "mem.swapin.average", "mem.swapinRate.average",
		"mem.swapout.average", "mem.swapoutRate.average", "mem.vmmemctl.average",
		"net.bytesRx.average", "net.bytesTx.average", "net.droppedRx.summation", "net.droppedTx.summation",
		"net.errorsRx.summation", "net.errorsTx.summation", "net.usage.average",
	}
)

// The pryorda series are built from the objects already read by the native
// collectors (one property read and one QueryPerf per entity for both
// families): host.go, vm.go and datastore.go add the few extra properties
// below and call the pryorda* emitters; perf.go adds the pryorda counters to
// its own query.
var (
	pryordaHostExtraProps = []string{"summary.config.name", "runtime.standbyMode"}
	pryordaVMExtraProps   = []string{"guest.toolsStatus"}
)

// pryordaVM is a VM kept for the pryorda emitter (phase 2: its labels come
// from its host, collected in parallel in phase 1).
type pryordaVM struct {
	vm *mo.VirtualMachine
	ds string
}

// pryordaHostLabels returns host_name / dc_name / cluster_name of a host.
func (r *Run) pryordaHostLabels(h *mo.HostSystem) L {
	name := strings.TrimRight(h.Summary.Config.Name, ".")
	if name == "" {
		name = h.Name
	}
	return L{"host_name", name, "dc_name", r.Topo.Datacenter(h.Self.Value), "cluster_name", r.Topo.ClusterOfHost(h.Self.Value)}
}

func (r *Run) setPryordaHost(moid string, l L) {
	r.mu.Lock()
	if r.pryHosts == nil {
		r.pryHosts = map[string]L{}
	}
	r.pryHosts[moid] = l
	r.mu.Unlock()
}

// addPryordaVM keeps a VM for collectPryorda; nil if pryorda skips it.
func (r *Run) addPryordaVM(vm *mo.VirtualMachine) *pryordaVM {
	if vm.Runtime.Host == nil || r.skipVM(vm) {
		return nil
	}
	ds := "n/a"
	if p := vm.Summary.Config.VmPathName; p != "" {
		ds = p
		if strings.HasPrefix(p, "[") {
			if end := strings.Index(p, "]"); end > 0 {
				ds = p[1:end]
			}
		}
	}
	pv := &pryordaVM{vm: vm, ds: ds}
	r.mu.Lock()
	r.pryVMs = append(r.pryVMs, pv)
	r.mu.Unlock()
	return pv
}

// labels of a VM: vm_name, ds_name, then its host's labels (n/a if unknown).
// Only valid in phase 2, once every host has been seen.
func (r *Run) pryordaVMLabels(pv *pryordaVM) L {
	hl, ok := r.pryHosts[pv.vm.Runtime.Host.Value]
	if !ok {
		hl = L{"host_name", "n/a", "dc_name", "n/a", "cluster_name", "n/a"}
	}
	return append(L{"vm_name", pv.vm.Name, "ds_name", pv.ds}, hl...)
}

// collectPryorda emits the VM series (phase 2). Host and datastore series are
// emitted directly by the host and datastore collectors.
func collectPryorda(_ context.Context, r *Run) error {
	for _, pv := range r.pryVMs {
		pryordaVMSeries(r, pv.vm, r.pryordaVMLabels(pv))
	}
	return nil
}

// skipVM applies the name filters and exclude_system_vms. Templates are kept:
// pryorda reports them (vmware_vm_template).
func (r *Run) skipVM(vm *mo.VirtualMachine) bool {
	if *r.Cfg.Filters.ExcludeSystemVMs && vm.Config != nil && vm.Config.ManagedBy != nil && vm.Config.ManagedBy.ExtensionKey != "" {
		return true
	}
	if r.vmExclude != nil && r.vmExclude.MatchString(vm.Name) {
		return true
	}
	return r.vmInclude != nil && !r.vmInclude.MatchString(vm.Name)
}

func epoch(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func pryordaHost(r *Run, h *mo.HostSystem, l L) {
	s := r.Sink
	with := func(kv ...string) L { return append(append(L{}, l...), kv...) }
	rt := h.Runtime
	on := rt.PowerState == types.HostSystemPowerStatePoweredOn
	s.Gauge("vmware_host_power_state", "VMWare Host Power state (On / Off)", b2f(on), l)
	s.Gauge("vmware_host_standby_mode", "VMWare Host Standby Mode (entering / exiting / in / none)",
		b2f(rt.StandbyMode == "in"), with("standby_mode_state", orUnknown(rt.StandbyMode)))
	s.Gauge("vmware_host_connection_state", "VMWare Host connection state (connected / disconnected / notResponding)",
		1, with("state", orUnknown(string(rt.ConnectionState))))
	s.Gauge("vmware_host_maintenance_mode", "VMWare Host maintenance mode (true / false)", b2f(rt.InMaintenanceMode), l)

	if on {
		if rt.BootTime != nil {
			s.Gauge("vmware_host_boot_timestamp_seconds", "VMWare Host boot time in seconds", epoch(*rt.BootTime), l)
		}
		qs := h.Summary.QuickStats
		s.Gauge("vmware_host_cpu_usage", "VMWare Host CPU usage in Mhz", float64(qs.OverallCpuUsage), l)
		s.Gauge("vmware_host_memory_usage", "VMWare Host Memory usage in Mbytes", float64(qs.OverallMemoryUsage), l)
		version, build, model, cpuModel := "unknown", "unknown", "unknown", "unknown"
		if p := h.Summary.Config.Product; p != nil {
			version, build = orUnknown(p.Version), orUnknown(p.Build)
		}
		if hw := h.Summary.Hardware; hw != nil {
			model, cpuModel = orUnknown(hw.Model), orUnknown(hw.CpuModel)
			if hw.NumCpuCores != 0 && hw.CpuMhz != 0 {
				s.Gauge("vmware_host_cpu_max", "VMWare Host CPU max availability in Mhz", float64(hw.NumCpuCores)*float64(hw.CpuMhz), l)
			}
			if hw.NumCpuCores != 0 {
				s.Gauge("vmware_host_num_cpu", "VMWare Number of processors in the Host", float64(hw.NumCpuCores), l)
			}
			s.Gauge("vmware_host_memory_max", "VMWare Host Memory Max availability in Mbytes", float64(hw.MemorySize)/1024/1024, l)
		}
		s.Gauge("vmware_host_product_info", `A metric with a constant "1" value labeled by version and build from os the host.`,
			1, with("version", version, "build", build))
		s.Gauge("vmware_host_hardware_info", `A metric with a constant "1" value labeled by model and cpu model from the host.`,
			1, with("hardware_model", model, "hardware_cpu_model", cpuModel))
	}

	hs := rt.HealthSystemRuntime
	if hs == nil {
		return
	}
	state := func(key string) float64 {
		switch strings.ToLower(key) {
		case "red":
			return 0
		case "yellow":
			return 1
		case "green":
			return 2
		}
		return 3
	}
	const stateHelp = "VMWare sensor state value (0=red / 1=yellow / 2=green / 3=unknown) labeled by sensor name and type from the host."
	if hs.SystemHealthInfo != nil {
		for _, sn := range hs.SystemHealthInfo.NumericSensorInfo {
			key := ""
			if sn.HealthState != nil {
				key = sn.HealthState.GetElementDescription().Key
			}
			s.Gauge("vmware_host_sensor_state", stateHelp, state(key), with("name", sn.Name, "type", sn.SensorType))
			val := float64(sn.CurrentReading) * math.Pow10(int(sn.UnitModifier))
			switch strings.ToLower(sn.BaseUnits) {
			case "rpm":
				s.Gauge("vmware_host_sensor_fan", "VMWare sensor fan speed value in RPM labeled by sensor name from the host.", val, with("name", sn.Name))
			case "degrees c":
				s.Gauge("vmware_host_sensor_temperature", "VMWare sensor temperature value in degree C labeled by sensor name from the host.", val, with("name", sn.Name))
			case "volts":
				s.Gauge("vmware_host_sensor_power_voltage", "VMWare sensor power voltage value in volt labeled by sensor name from the host.", val, with("name", sn.Name))
			case "amps":
				s.Gauge("vmware_host_sensor_power_current", "VMWare sensor power current value in amp labeled by sensor name from the host.", val, with("name", sn.Name))
			case "watts":
				s.Gauge("vmware_host_sensor_power_watt", "VMWare sensor power watt value in watt labeled by sensor name from the host.", val, with("name", sn.Name))
			case "redundancy-discrete":
				s.Gauge("vmware_host_sensor_redundancy", "VMWare sensor redundancy value (1=ok / 0=ko) labeled by sensor name from the host.", float64(sn.CurrentReading), with("name", sn.Name))
			}
		}
	}
	if hw := hs.HardwareStatusInfo; hw != nil {
		for _, it := range append(append([]types.BaseHostHardwareElementInfo{}, hw.CpuStatusInfo...), hw.MemoryStatusInfo...) {
			e := it.GetHostHardwareElementInfo()
			key := ""
			if e.Status != nil {
				key = e.Status.GetElementDescription().Key
			}
			s.Gauge("vmware_host_sensor_state", stateHelp, state(key), with("name", e.Name, "type", "n/a"))
		}
	}
}

func pryordaVMSeries(r *Run, vm *mo.VirtualMachine, l L) {
	s := r.Sink
	with := func(kv ...string) L { return append(append(L{}, l...), kv...) }
	sc := vm.Summary.Config
	on := vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn

	// section "vms"
	s.Gauge("vmware_vm_power_state", "VMWare VM Power state (On / Off)", b2f(on), l)
	if on && vm.Runtime.BootTime != nil {
		s.Gauge("vmware_vm_boot_timestamp_seconds", "VMWare VM boot time in seconds", epoch(*vm.Runtime.BootTime), l)
	}
	s.Gauge("vmware_vm_num_cpu", "VMWare Number of processors in the virtual machine", float64(sc.NumCpu), l)
	s.Gauge("vmware_vm_memory_max", "VMWare VM Memory Max availability in Mbytes", float64(sc.MemorySizeMB), l)
	s.Gauge("vmware_vm_max_cpu_usage", "VMWare VM Cpu Max availability in hz", float64(vm.Runtime.MaxCpuUsage), l)
	s.Gauge("vmware_vm_template", "VMWare VM Template (true / false)", b2f(sc.Template), l)

	// section "vmguests"
	if g := vm.Guest; g != nil {
		for _, d := range g.Disk {
			s.Gauge("vmware_vm_guest_disk_free", "Disk metric per partition", float64(d.FreeSpace), with("partition", d.DiskPath))
			s.Gauge("vmware_vm_guest_disk_capacity", "Disk capacity metric per partition", float64(d.Capacity), with("partition", d.DiskPath))
		}
		if g.ToolsStatus != "" {
			s.Gauge("vmware_vm_guest_tools_running_status", "VM tools running status", 1, with("tools_status", string(g.ToolsStatus)))
		}
		if g.ToolsVersion != "" {
			s.Gauge("vmware_vm_guest_tools_version", "VM tools version", 1, with("tools_version", g.ToolsVersion))
		}
		if g.ToolsVersionStatus2 != "" {
			s.Gauge("vmware_vm_guest_tools_version_status", "VM tools version status", 1, with("tools_version_status", g.ToolsVersionStatus2))
		}
	}

	// section "snapshots": absent (not 0) for VMs without snapshots, as in pryorda
	if vm.Snapshot != nil {
		count := 0
		var walk func([]types.VirtualMachineSnapshotTree)
		walk = func(list []types.VirtualMachineSnapshotTree) {
			for _, sn := range list {
				count++
				s.Gauge("vmware_vm_snapshot_timestamp_seconds", "VMWare Snapshot creation time in seconds", epoch(sn.CreateTime), with("vm_snapshot_name", sn.Name))
				walk(sn.ChildSnapshotList)
			}
		}
		walk(vm.Snapshot.RootSnapshotList)
		s.Gauge("vmware_vm_snapshots", "VMWare current number of existing snapshots", float64(count), l)
	}
}

// pryordaDatastore emits the series of one datastore (called by collectDatastores).
func pryordaDatastore(r *Run, ds *mo.Datastore, dc, pod string) {
	s := r.Sink
	l := L{"ds_name", ds.Name, "dc_name", dc, "ds_cluster", pod}
	sm := ds.Summary
	s.Gauge("vmware_datastore_capacity_size", "VMWare Datasore capacity in bytes", float64(sm.Capacity), l)
	s.Gauge("vmware_datastore_freespace_size", "VMWare Datastore freespace in bytes", float64(sm.FreeSpace), l)
	s.Gauge("vmware_datastore_uncommited_size", "VMWare Datastore uncommitted in bytes", float64(sm.Uncommitted), l)
	s.Gauge("vmware_datastore_provisoned_size", "VMWare Datastore provisoned in bytes", float64(sm.Capacity-sm.FreeSpace+sm.Uncommitted), l)
	s.Gauge("vmware_datastore_hosts", "VMWare Hosts number using this datastore", float64(len(ds.Host)), l)
	s.Gauge("vmware_datastore_vms", "VMWare Virtual Machines count per datastore", float64(len(ds.Vm)), l)
	s.Gauge("vmware_datastore_maintenance_mode", "VMWare datastore maintenance mode (normal / inMaintenance / enteringMaintenance)",
		1, append(append(L{}, l...), "mode", orUnknown(sm.MaintenanceMode)))
	typ := sm.Type
	if typ == "" {
		typ = "normal"
	}
	s.Gauge("vmware_datastore_type", "VMWare datastore type (VMFS, NetworkFileSystem, NetworkFileSystem41, CIFS, VFAT, VSAN, VFFS)",
		1, append(append(L{}, l...), "ds_type", typ))
	s.Gauge("vmware_datastore_accessible", "VMWare datastore accessible (true / false)", b2f(sm.Accessible), l)
}
