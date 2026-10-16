package collector

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/vmware/govmomi/performance"
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

var pryordaHostProps = []string{
	"name", "summary.config.name", "summary.config.product", "summary.hardware", "summary.quickStats",
	"runtime.powerState", "runtime.standbyMode", "runtime.connectionState", "runtime.inMaintenanceMode", "runtime.bootTime",
}

var pryordaVMProps = []string{
	"name", "config.managedBy", "summary.config", "runtime.host", "runtime.powerState", "runtime.bootTime", "runtime.maxCpuUsage",
	"guest.disk", "guest.toolsStatus", "guest.toolsVersion", "guest.toolsVersionStatus2", "snapshot",
}

func collectPryorda(ctx context.Context, r *Run) error {
	// Hosts first: VMs inherit host_name / dc_name / cluster_name from their host.
	props := append([]string{}, pryordaHostProps...)
	if *r.Cfg.Host.Sensors {
		props = append(props, "runtime.healthSystemRuntime")
	}
	var hosts []mo.HostSystem
	if err := r.retrieve(ctx, []string{"HostSystem"}, props, &hosts); err != nil {
		return err
	}
	hostLabels := map[string]L{}
	var perfHosts []perfEntity
	for i := range hosts {
		h := &hosts[i]
		if r.hostExclude != nil && r.hostExclude.MatchString(h.Name) {
			continue
		}
		name := strings.TrimRight(h.Summary.Config.Name, ".")
		if name == "" {
			name = h.Name
		}
		l := L{"host_name", name, "dc_name", r.Topo.Datacenter(h.Self.Value), "cluster_name", r.Topo.ClusterOfHost(h.Self.Value)}
		hostLabels[h.Self.Value] = l
		pryordaHost(r, h, l)
		if h.Runtime.PowerState == types.HostSystemPowerStatePoweredOn {
			perfHosts = append(perfHosts, perfEntity{ref: h.Self, labels: l})
		}
	}

	var vms []mo.VirtualMachine
	if err := r.retrieve(ctx, []string{"VirtualMachine"}, pryordaVMProps, &vms); err != nil {
		return err
	}
	var perfVMs []perfEntity
	for i := range vms {
		vm := &vms[i]
		if vm.Runtime.Host == nil || r.skipVM(vm) {
			continue
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
		hl, ok := hostLabels[vm.Runtime.Host.Value]
		if !ok {
			hl = L{"host_name", "n/a", "dc_name", "n/a", "cluster_name", "n/a"}
		}
		l := append(L{"vm_name", vm.Name, "ds_name", ds}, hl...)
		pryordaVM(r, vm, l)
		if vm.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
			perfVMs = append(perfVMs, perfEntity{ref: vm.Self, labels: l})
		}
	}

	if err := pryordaDatastores(ctx, r); err != nil {
		return err
	}
	if !*r.Cfg.Collectors.Perf {
		return nil
	}
	pm := performance.NewManager(r.Vim)
	byName, err := pm.CounterInfoByName(ctx)
	if err != nil {
		return err
	}
	errH := pryordaPerf(ctx, r, pm, byName, "host", perfHosts, pryordaHostCounters)
	errV := pryordaPerf(ctx, r, pm, byName, "vm", perfVMs, pryordaVMCounters)
	if errH != nil {
		return errH
	}
	r.Log.Debug("pryorda compatibility metrics collected", "hosts", len(hostLabels), "powered_on_vms", len(perfVMs))
	return errV
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

func pryordaVM(r *Run, vm *mo.VirtualMachine, l L) {
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

func pryordaDatastores(ctx context.Context, r *Run) error {
	var dss []mo.Datastore
	if err := r.retrieve(ctx, []string{"Datastore"}, []string{"name", "parent", "summary", "host", "vm"}, &dss); err != nil {
		return err
	}
	s := r.Sink
	for _, ds := range dss {
		if r.dsExclude != nil && r.dsExclude.MatchString(ds.Name) {
			continue
		}
		dc, pod := "", ""
		if ds.Parent != nil {
			dc = r.Topo.Datacenter(ds.Parent.Value)
			if ds.Parent.Type == "StoragePod" {
				pod = r.Topo.Name[ds.Parent.Value]
			}
		}
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
	return nil
}

// pryordaPerf queries the latest real-time sample (aggregate instance) of each
// counter, without unit conversion, as vmware_<kind>_<group>_<name>_<rollup>.
func pryordaPerf(ctx context.Context, r *Run, pm *performance.Manager, byName map[string]*types.PerfCounterInfo,
	kind string, ents []perfEntity, counters []string) error {
	if len(ents) == 0 {
		return nil
	}
	metricOf := map[int32]string{}
	var ids []types.PerfMetricId
	for _, c := range counters {
		info, ok := byName[c]
		if !ok {
			r.Log.Debug("performance counter unknown on this vCenter, skipped", "counter", c)
			continue
		}
		metricOf[info.Key] = "vmware_" + kind + "_" + strings.ReplaceAll(c, ".", "_")
		ids = append(ids, types.PerfMetricId{CounterId: info.Key, Instance: ""})
	}
	if len(ids) == 0 {
		return nil
	}
	labelsOf := map[string]L{}
	for _, e := range ents {
		labelsOf[e.ref.Value] = e.labels
	}
	spec := func(e perfEntity) types.PerfQuerySpec {
		return types.PerfQuerySpec{Entity: e.ref, MetricId: ids, IntervalId: 20, MaxSample: 1, Format: string(types.PerfFormatNormal)}
	}
	var firstErr error
	bs := r.Cfg.Perf.BatchSize
	for i := 0; i < len(ents); i += bs {
		batch := ents[i:min(i+bs, len(ents))]
		specs := make([]types.PerfQuerySpec, 0, len(batch))
		for _, e := range batch {
			specs = append(specs, spec(e))
		}
		res, err := pm.Query(ctx, specs)
		if err != nil && len(specs) > 1 {
			// An entity that vanished fails the whole batch: retry one at a time.
			res, err = nil, nil
			for _, sp := range specs {
				one, e := pm.Query(ctx, []types.PerfQuerySpec{sp})
				if e != nil {
					err = e
					continue
				}
				res = append(res, one...)
			}
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for _, base := range res {
			em, ok := base.(*types.PerfEntityMetric)
			if !ok {
				continue
			}
			for _, sb := range em.Value {
				series, ok := sb.(*types.PerfMetricIntSeries)
				if !ok || series.Id.Instance != "" {
					continue
				}
				name, ok := metricOf[series.Id.CounterId]
				if !ok {
					continue
				}
				// pryorda reports float(sum(values)), 0 for an empty series;
				// -1 (no sample) is skipped rather than summed.
				sum := 0.0
				for _, v := range series.Value {
					if v >= 0 {
						sum += float64(v)
					}
				}
				r.Sink.Gauge(name, name, sum, labelsOf[em.Entity.Value])
			}
		}
	}
	return firstErr
}
