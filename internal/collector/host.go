package collector

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"math"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var hostBaseProps = []string{
	"name", "parent", "overallStatus", "configIssue",
	"runtime.connectionState", "runtime.powerState", "runtime.inMaintenanceMode",
	"runtime.inQuarantineMode", "runtime.bootTime", "runtime.dasHostState",
	"summary.hardware", "summary.quickStats", "summary.config.product", "summary.rebootRequired",
	"hardware.biosInfo", "config.certificate", "config.network", "config.service",
	"vm", "datastore",
}

func collectHosts(ctx context.Context, r *Run) error {
	props := append([]string{}, hostBaseProps...)
	if *r.Cfg.Host.Sensors {
		props = append(props, "runtime.healthSystemRuntime")
	}
	if *r.Cfg.Host.StoragePaths {
		props = append(props, "config.storageDevice.multipathInfo")
	}
	var hosts []mo.HostSystem
	if err := r.retrieve(ctx, []string{"HostSystem"}, props, &hosts); err != nil {
		return err
	}
	wantSvc := map[string]bool{}
	for _, sname := range r.Cfg.Host.Services {
		wantSvc[sname] = true
	}
	emit := *r.Cfg.Collectors.Host
	s := r.Sink
	for i := range hosts {
		h := &hosts[i]
		if r.hostExclude != nil && r.hostExclude.MatchString(h.Name) {
			continue
		}
		dc := r.Topo.Datacenter(h.Self.Value)
		cl := r.Topo.ClusterOfHost(h.Self.Value)
		l := L{"datacenter", dc, "cluster", cl, "host", h.Name}

		if h.Runtime.ConnectionState == types.HostSystemConnectionStateConnected && h.Runtime.PowerState == types.HostSystemPowerStatePoweredOn {
			r.addPerfHost(perfEntity{ref: h.Self, labels: l})
		}
		if !emit {
			continue
		}

		s.Gauge(n("host_overall_status"), "Statut global de l'ESX"+statusHelp+".", statusValue(h.OverallStatus), l)
		s.Gauge(n("host_connection_state"), "État de connexion de l'ESX au vCenter"+hostConnHelp+".", hostConn(h.Runtime.ConnectionState), l)
		s.Gauge(n("host_power_state"), "État d'alimentation de l'ESX"+powerHelp+".", hostPower(h.Runtime.PowerState), l)
		s.Gauge(n("host_maintenance_mode"), "1 si l'ESX est en mode maintenance.", b2f(h.Runtime.InMaintenanceMode), l)
		s.Gauge(n("host_quarantine_mode"), "1 si l'ESX est en quarantaine (Proactive HA).", bp2f(h.Runtime.InQuarantineMode), l)
		s.Gauge(n("host_config_issues"), "Nombre de problèmes de configuration signalés sur l'ESX.", float64(len(h.ConfigIssue)), l)
		s.Gauge(n("host_reboot_required"), "1 si un redémarrage de l'ESX est requis.", b2f(h.Summary.RebootRequired), l)
		s.Gauge(n("host_vms"), "Nombre de VMs enregistrées sur l'ESX.", float64(len(h.Vm)), l)
		s.Gauge(n("host_datastores"), "Nombre de datastores montés sur l'ESX.", float64(len(h.Datastore)), l)
		if h.Runtime.BootTime != nil {
			s.Gauge(n("host_boot_timestamp_seconds"), "Horodatage du dernier démarrage de l'ESX.", float64(h.Runtime.BootTime.Unix()), l)
		}
		if das := h.Runtime.DasHostState; das != nil {
			s.Gauge(n("host_ha_state"), "État de l'agent HA (FDM) de l'ESX, valeur 1 pour l'état courant.", 1, append(append(L{}, l...), "state", das.State))
		}

		info := L{"datacenter", dc, "cluster", cl, "host", h.Name}
		if p := h.Summary.Config.Product; p != nil {
			info = append(info, "version", p.Version, "build", p.Build, "product", p.FullName)
		} else {
			info = append(info, "version", "", "build", "", "product", "")
		}
		if hw := h.Summary.Hardware; hw != nil {
			info = append(info, "vendor", hw.Vendor, "model", hw.Model, "cpu_model", hw.CpuModel)
		} else {
			info = append(info, "vendor", "", "model", "", "cpu_model", "")
		}
		bios := ""
		if h.Hardware != nil && h.Hardware.BiosInfo != nil {
			bios = h.Hardware.BiosInfo.BiosVersion
		}
		info = append(info, "bios_version", bios)
		s.Gauge(n("host_info"), "Informations sur l'ESX (valeur toujours 1).", 1, info)

		if hw := h.Summary.Hardware; hw != nil {
			s.Gauge(n("host_cpu_cores"), "Nombre de coeurs physiques.", float64(hw.NumCpuCores), l)
			s.Gauge(n("host_cpu_threads"), "Nombre de threads physiques.", float64(hw.NumCpuThreads), l)
			s.Gauge(n("host_cpu_total_mhz"), "Capacité CPU totale (MHz).", float64(hw.CpuMhz)*float64(hw.NumCpuCores), l)
			s.Gauge(n("host_memory_total_bytes"), "Mémoire physique totale.", float64(hw.MemorySize), l)
		}
		qs := h.Summary.QuickStats
		s.Gauge(n("host_cpu_usage_mhz"), "CPU consommé (MHz).", float64(qs.OverallCpuUsage), l)
		s.Gauge(n("host_memory_usage_bytes"), "Mémoire consommée.", float64(qs.OverallMemoryUsage)*1024*1024, l)
		s.Gauge(n("host_uptime_seconds"), "Uptime de l'ESX.", float64(qs.Uptime), l)

		if h.Config != nil {
			hostCertificate(r, h, l)
			hostNICs(r, h, l)
			if h.Config.Service != nil {
				for _, svc := range h.Config.Service.Service {
					if !wantSvc[svc.Key] {
						continue
					}
					sl := append(append(L{}, l...), "service", svc.Key, "policy", svc.Policy)
					s.Gauge(n("host_service_running"), "1 si le service ESX est démarré (TSM-SSH, ntpd, ...).", b2f(svc.Running), sl)
				}
			}
			if *r.Cfg.Host.StoragePaths && h.Config.StorageDevice != nil && h.Config.StorageDevice.MultipathInfo != nil {
				hostPaths(r, h.Config.StorageDevice.MultipathInfo, l)
			}
		}
		if *r.Cfg.Host.Sensors && h.Runtime.HealthSystemRuntime != nil {
			hostSensors(r, h.Runtime.HealthSystemRuntime, l)
		}
	}
	return nil
}

func hostCertificate(r *Run, h *mo.HostSystem, l L) {
	if len(h.Config.Certificate) == 0 {
		return
	}
	raw := h.Config.Certificate
	if blk, _ := pem.Decode(raw); blk != nil {
		raw = blk.Bytes
	}
	c, err := x509.ParseCertificate(raw)
	if err != nil {
		return
	}
	r.Sink.Gauge(n("host_certificate_expiry_timestamp_seconds"), "Date d'expiration du certificat de l'ESX (epoch).", float64(c.NotAfter.Unix()), l)
}

func hostNICs(r *Run, h *mo.HostSystem, l L) {
	nw := h.Config.Network
	if nw == nil {
		return
	}
	used := map[string]bool{}
	for _, vs := range nw.Vswitch {
		for _, k := range vs.Pnic {
			used[k] = true
		}
	}
	for _, ps := range nw.ProxySwitch {
		for _, k := range ps.Pnic {
			used[k] = true
		}
	}
	for _, p := range nw.Pnic {
		up, speed := 0.0, 0.0
		if p.LinkSpeed != nil {
			up = 1
			speed = float64(p.LinkSpeed.SpeedMb)
		}
		u := "false"
		if used[p.Key] {
			u = "true"
		}
		nl := append(append(L{}, l...), "nic", p.Device, "used", u, "driver", p.Driver)
		r.Sink.Gauge(n("host_pnic_link_up"), "1 si le lien de la carte réseau physique est actif. used=true si elle est uplink d'un vSwitch/vDS.", up, nl)
		r.Sink.Gauge(n("host_pnic_speed_mbps"), "Vitesse négociée de la carte réseau physique (Mb/s, 0 si lien down).", speed, nl)
	}
}

func hostPaths(r *Run, mp *types.HostMultipathInfo, l L) {
	states := map[string]int{"active": 0, "standby": 0, "disabled": 0, "dead": 0, "unknown": 0}
	noPath, single := 0, 0
	for _, lun := range mp.Lun {
		alive := 0
		for _, p := range lun.Path {
			st := strings.ToLower(p.PathState)
			if _, ok := states[st]; !ok {
				st = "unknown"
			}
			states[st]++
			if st == "active" || st == "standby" {
				alive++
			}
		}
		switch {
		case alive == 0:
			noPath++
		case alive == 1 && len(lun.Path) > 1:
			single++
		}
	}
	for st, c := range states {
		r.Sink.Gauge(n("host_storage_paths"), "Nombre de chemins de stockage par état (active, standby, dead...).", float64(c), append(append(L{}, l...), "state", st))
	}
	r.Sink.Gauge(n("host_storage_luns"), "Nombre de LUN vus par l'ESX.", float64(len(mp.Lun)), l)
	r.Sink.Gauge(n("host_storage_luns_no_path"), "LUN sans aucun chemin vivant (perte d'accès).", float64(noPath), l)
	r.Sink.Gauge(n("host_storage_luns_degraded"), "LUN multi-chemins n'ayant plus qu'un seul chemin vivant (perte de redondance).", float64(single), l)
}

func hostSensors(r *Run, hs *types.HealthSystemRuntime, l L) {
	if hs.SystemHealthInfo != nil {
		for _, sn := range hs.SystemHealthInfo.NumericSensorInfo {
			health := ""
			if hs := sn.HealthState; hs != nil {
				health = hs.GetElementDescription().Key
			}
			sl := append(append(L{}, l...), "sensor", sn.Name, "type", sn.SensorType)
			r.Sink.Gauge(n("host_sensor_health"), "Santé d'un capteur matériel IPMI/CIM"+statusHelp+".", healthString(health), sl)
			unit := sn.BaseUnits
			if unit == "" {
				unit = "none"
			}
			val := float64(sn.CurrentReading) * math.Pow10(int(sn.UnitModifier))
			r.Sink.Gauge(n("host_sensor_value"), "Valeur courante d'un capteur matériel (unité dans le label unit).", val, append(sl, "unit", unit))
		}
	}
	if hw := hs.HardwareStatusInfo; hw != nil {
		comp := func(kind string, items []types.BaseHostHardwareElementInfo) {
			for _, it := range items {
				e := it.GetHostHardwareElementInfo()
				st := ""
				if e.Status != nil {
					st = e.Status.GetElementDescription().Key
				}
				r.Sink.Gauge(n("host_hardware_status"), "Statut d'un composant matériel (mémoire, cpu, stockage)"+statusHelp+".",
					healthString(st), append(append(L{}, l...), "component", kind, "name", e.Name))
			}
		}
		comp("memory", hw.MemoryStatusInfo)
		comp("cpu", hw.CpuStatusInfo)
		storage := make([]types.BaseHostHardwareElementInfo, 0, len(hw.StorageStatusInfo))
		for i := range hw.StorageStatusInfo {
			storage = append(storage, &hw.StorageStatusInfo[i])
		}
		comp("storage", storage)
	}
}
