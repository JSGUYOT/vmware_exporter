package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func collectDatacenters(ctx context.Context, r *Run) error {
	var dcs []mo.Datacenter
	if err := r.retrieve(ctx, []string{"Datacenter"}, []string{"name", "overallStatus"}, &dcs); err != nil {
		return err
	}
	for _, dc := range dcs {
		l := L{"datacenter", dc.Name}
		r.Sink.Gauge(n("datacenter_overall_status"), "Statut global du datacenter"+statusHelp+".", statusValue(dc.OverallStatus), l)
	}
	return nil
}

// Propriétés cluster présentes à l'identique en vSphere 8.x et 9.x.
// Volontairement absentes : propriétés vCLS (dépréciées en 9.0, désactivées en
// 9.1), SIOC / Storage DRS load balancer (retirés en 9.0), Host Profiles
// (dépréciés en 9.0) et baselines vLCM (retirées en 9.0).
var clusterProps = []string{
	"name", "parent", "overallStatus", "configIssue",
	"summary", "configurationEx", "host", "datastore", "drsRecommendation",
}

func collectClusters(ctx context.Context, r *Run) error {
	var cls []mo.ClusterComputeResource
	if err := r.retrieve(ctx, []string{"ClusterComputeResource"}, clusterProps, &cls); err != nil {
		return err
	}
	s := r.Sink
	for _, c := range cls {
		dc := r.Topo.Datacenter(c.Self.Value)
		l := L{"datacenter", dc, "cluster", c.Name}
		s.Gauge(n("cluster_overall_status"), "Statut global du cluster"+statusHelp+".", statusValue(c.OverallStatus), l)
		s.Gauge(n("cluster_config_issues"), "Nombre de problèmes de configuration signalés sur le cluster.", float64(len(c.ConfigIssue)), l)
		s.Gauge(n("cluster_hosts"), "Nombre d'ESX dans le cluster.", float64(len(c.Host)), l)
		s.Gauge(n("cluster_datastores"), "Nombre de datastores montés sur le cluster.", float64(len(c.Datastore)), l)
		s.Gauge(n("cluster_drs_recommendations"), "Nombre de recommandations DRS en attente.", float64(len(c.DrsRecommendation)), l)

		if cs, ok := c.Summary.(*types.ClusterComputeResourceSummary); ok && cs != nil {
			s.Gauge(n("cluster_hosts_effective"), "Nombre d'ESX effectifs (connectés, hors maintenance).", float64(cs.NumEffectiveHosts), l)
			s.Gauge(n("cluster_cpu_total_mhz"), "Capacité CPU totale du cluster (MHz).", float64(cs.TotalCpu), l)
			s.Gauge(n("cluster_cpu_effective_mhz"), "Capacité CPU effective disponible pour les VMs (MHz).", float64(cs.EffectiveCpu), l)
			s.Gauge(n("cluster_memory_total_bytes"), "Mémoire totale du cluster.", float64(cs.TotalMemory), l)
			s.Gauge(n("cluster_memory_effective_bytes"), "Mémoire effective disponible pour les VMs.", float64(cs.EffectiveMemory)*1024*1024, l)
			s.Gauge(n("cluster_cpu_cores"), "Nombre de coeurs physiques.", float64(cs.NumCpuCores), l)
			s.Gauge(n("cluster_cpu_threads"), "Nombre de threads physiques.", float64(cs.NumCpuThreads), l)
			s.Counter(n("cluster_vmotions_total"), "Nombre de vMotions effectués dans le cluster.", float64(cs.NumVmotions), l)
			if cs.DrsScore != 0 {
				s.Gauge(n("cluster_drs_score"), "Score DRS du cluster (0-100, santé des VMs).", float64(cs.DrsScore), l)
			}
			if u := cs.UsageSummary; u != nil {
				s.Gauge(n("cluster_cpu_demand_mhz"), "Demande CPU des VMs (MHz).", float64(u.CpuDemandMhz), l)
				s.Gauge(n("cluster_cpu_entitled_mhz"), "CPU attribué aux VMs (MHz).", float64(u.CpuEntitledMhz), l)
				s.Gauge(n("cluster_cpu_reserved_mhz"), "CPU réservé (MHz).", float64(u.CpuReservationMhz), l)
				s.Gauge(n("cluster_memory_demand_bytes"), "Demande mémoire des VMs.", float64(u.MemDemandMB)*1024*1024, l)
				s.Gauge(n("cluster_memory_entitled_bytes"), "Mémoire attribuée aux VMs.", float64(u.MemEntitledMB)*1024*1024, l)
				s.Gauge(n("cluster_memory_reserved_bytes"), "Mémoire réservée.", float64(u.MemReservationMB)*1024*1024, l)
				s.Gauge(n("cluster_vms_powered_on"), "Nombre de VMs allumées.", float64(u.TotalVmCount-u.PoweredOffVmCount), l)
				s.Gauge(n("cluster_vms"), "Nombre total de VMs.", float64(u.TotalVmCount), l)
			}
			if ac, ok := cs.AdmissionControlInfo.(*types.ClusterFailoverLevelAdmissionControlInfo); ok && ac != nil {
				s.Gauge(n("cluster_ha_failover_level_current"), "Nombre de pannes d'ESX actuellement tolérées (politique 'nombre d'ESX', slots).", float64(ac.CurrentFailoverLevel), l)
			}
			if ac, ok := cs.AdmissionControlInfo.(*types.ClusterFailoverResourcesAdmissionControlInfo); ok && ac != nil {
				s.Gauge(n("cluster_ha_failover_cpu_resources_percent"), "Ressources CPU de basculement HA disponibles (%).", float64(ac.CurrentCpuFailoverResourcesPercent), l)
				s.Gauge(n("cluster_ha_failover_memory_resources_percent"), "Ressources mémoire de basculement HA disponibles (%).", float64(ac.CurrentMemoryFailoverResourcesPercent), l)
			}
		}
		if cfg, ok := c.ConfigurationEx.(*types.ClusterConfigInfoEx); ok && cfg != nil {
			s.Gauge(n("cluster_ha_enabled"), "1 si vSphere HA est activé.", bp2f(cfg.DasConfig.Enabled), l)
			s.Gauge(n("cluster_ha_admission_control_enabled"), "1 si le contrôle d'admission HA est activé.", bp2f(cfg.DasConfig.AdmissionControlEnabled), l)
			haAC := bp2f(cfg.DasConfig.Enabled) == 1 && bp2f(cfg.DasConfig.AdmissionControlEnabled) == 1
			policy := "none"
			switch p := cfg.DasConfig.AdmissionControlPolicy.(type) {
			case *types.ClusterFailoverLevelAdmissionControlPolicy:
				policy = "failover_level"
				if haAC && p != nil {
					s.Gauge(n("cluster_ha_failover_level_configured"), "Nombre de pannes d'ESX à tolérer configuré (politique 'nombre d'ESX').", float64(p.FailoverLevel), l)
				}
			case *types.ClusterFailoverResourcesAdmissionControlPolicy:
				policy = "resources_percent"
				if haAC && p != nil {
					s.Gauge(n("cluster_ha_failover_cpu_resources_configured_percent"), "Pourcentage CPU réservé au basculement HA (configuré ou calculé).", float64(p.CpuFailoverResourcesPercent), l)
					s.Gauge(n("cluster_ha_failover_memory_resources_configured_percent"), "Pourcentage mémoire réservé au basculement HA (configuré ou calculé).", float64(p.MemoryFailoverResourcesPercent), l)
				}
			case *types.ClusterFailoverHostAdmissionControlPolicy:
				policy = "failover_hosts"
			}
			s.Gauge(n("cluster_ha_admission_control_policy"), "Politique de contrôle d'admission HA (label policy, valeur 1).", 1, append(append(L{}, l...), "policy", policy))
			s.Gauge(n("cluster_ha_host_monitoring_enabled"), "1 si la surveillance des ESX par HA est activée.", b2f(cfg.DasConfig.HostMonitoring == "enabled"), l)
			s.Gauge(n("cluster_drs_enabled"), "1 si DRS est activé.", bp2f(cfg.DrsConfig.Enabled), l)
			s.Gauge(n("cluster_drs_fully_automated"), "1 si DRS est en mode entièrement automatisé.", b2f(cfg.DrsConfig.DefaultVmBehavior == types.DrsBehaviorFullyAutomated), l)
			vsanOn := false
			if cfg.VsanConfigInfo != nil {
				vsanOn = bp2f(cfg.VsanConfigInfo.Enabled) == 1
			}
			s.Gauge(n("cluster_vsan_enabled"), "1 si vSAN est activé sur le cluster.", b2f(vsanOn), l)
			if cfg.DpmConfigInfo != nil {
				s.Gauge(n("cluster_dpm_enabled"), "1 si DPM (gestion d'énergie) est activé.", bp2f(cfg.DpmConfigInfo.Enabled), l)
			}
		}
	}
	return nil
}
