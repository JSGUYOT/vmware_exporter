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
		r.Sink.Gauge(n("datacenter_overall_status"), "Overall status of the datacenter"+statusHelp+".", statusValue(dc.OverallStatus), l)
	}
	return nil
}

// Cluster properties that are identical in vSphere 8.x and 9.x.
// Deliberately left out: vCLS properties (deprecated in 9.0, disabled in 9.1),
// SIOC / Storage DRS load balancer (removed in 9.0), Host Profiles
// (deprecated in 9.0) and vLCM baselines (removed in 9.0).
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
		s.Gauge(n("cluster_overall_status"), "Overall status of the cluster"+statusHelp+".", statusValue(c.OverallStatus), l)
		s.Gauge(n("cluster_config_issues"), "Number of configuration issues reported on the cluster.", float64(len(c.ConfigIssue)), l)
		s.Gauge(n("cluster_hosts"), "Number of hosts in the cluster.", float64(len(c.Host)), l)
		s.Gauge(n("cluster_datastores"), "Number of datastores mounted on the cluster.", float64(len(c.Datastore)), l)
		s.Gauge(n("cluster_drs_recommendations"), "Number of pending DRS recommendations.", float64(len(c.DrsRecommendation)), l)

		if cs, ok := c.Summary.(*types.ClusterComputeResourceSummary); ok && cs != nil {
			s.Gauge(n("cluster_hosts_effective"), "Number of effective hosts (connected, not in maintenance).", float64(cs.NumEffectiveHosts), l)
			s.Gauge(n("cluster_cpu_total_mhz"), "Total CPU capacity of the cluster (MHz).", float64(cs.TotalCpu), l)
			s.Gauge(n("cluster_cpu_effective_mhz"), "Effective CPU capacity available to VMs (MHz).", float64(cs.EffectiveCpu), l)
			s.Gauge(n("cluster_memory_total_bytes"), "Total memory of the cluster.", float64(cs.TotalMemory), l)
			s.Gauge(n("cluster_memory_effective_bytes"), "Effective memory available to VMs.", float64(cs.EffectiveMemory)*1024*1024, l)
			s.Gauge(n("cluster_cpu_cores"), "Number of physical cores.", float64(cs.NumCpuCores), l)
			s.Gauge(n("cluster_cpu_threads"), "Number of physical threads.", float64(cs.NumCpuThreads), l)
			s.Counter(n("cluster_vmotions_total"), "Number of vMotions performed in the cluster.", float64(cs.NumVmotions), l)
			if cs.DrsScore != 0 {
				s.Gauge(n("cluster_drs_score"), "Cluster DRS score (0-100, VM happiness).", float64(cs.DrsScore), l)
			}
			if u := cs.UsageSummary; u != nil {
				s.Gauge(n("cluster_cpu_demand_mhz"), "CPU demand of VMs (MHz).", float64(u.CpuDemandMhz), l)
				s.Gauge(n("cluster_cpu_entitled_mhz"), "CPU entitled to VMs (MHz).", float64(u.CpuEntitledMhz), l)
				s.Gauge(n("cluster_cpu_reserved_mhz"), "Reserved CPU (MHz).", float64(u.CpuReservationMhz), l)
				s.Gauge(n("cluster_memory_demand_bytes"), "Memory demand of VMs.", float64(u.MemDemandMB)*1024*1024, l)
				s.Gauge(n("cluster_memory_entitled_bytes"), "Memory entitled to VMs.", float64(u.MemEntitledMB)*1024*1024, l)
				s.Gauge(n("cluster_memory_reserved_bytes"), "Reserved memory.", float64(u.MemReservationMB)*1024*1024, l)
				s.Gauge(n("cluster_vms_powered_on"), "Number of powered-on VMs.", float64(u.TotalVmCount-u.PoweredOffVmCount), l)
				s.Gauge(n("cluster_vms"), "Total number of VMs.", float64(u.TotalVmCount), l)
			}
			if ac, ok := cs.AdmissionControlInfo.(*types.ClusterFailoverLevelAdmissionControlInfo); ok && ac != nil {
				s.Gauge(n("cluster_ha_failover_level_current"), "Number of host failures currently tolerated ('host failures' policy, slots).", float64(ac.CurrentFailoverLevel), l)
			}
			if ac, ok := cs.AdmissionControlInfo.(*types.ClusterFailoverResourcesAdmissionControlInfo); ok && ac != nil {
				s.Gauge(n("cluster_ha_failover_cpu_resources_percent"), "Available HA failover CPU resources (%).", float64(ac.CurrentCpuFailoverResourcesPercent), l)
				s.Gauge(n("cluster_ha_failover_memory_resources_percent"), "Available HA failover memory resources (%).", float64(ac.CurrentMemoryFailoverResourcesPercent), l)
			}
		}
		if cfg, ok := c.ConfigurationEx.(*types.ClusterConfigInfoEx); ok && cfg != nil {
			s.Gauge(n("cluster_ha_enabled"), "1 if vSphere HA is enabled.", bp2f(cfg.DasConfig.Enabled), l)
			s.Gauge(n("cluster_ha_admission_control_enabled"), "1 if HA admission control is enabled.", bp2f(cfg.DasConfig.AdmissionControlEnabled), l)
			haAC := bp2f(cfg.DasConfig.Enabled) == 1 && bp2f(cfg.DasConfig.AdmissionControlEnabled) == 1
			policy := "none"
			switch p := cfg.DasConfig.AdmissionControlPolicy.(type) {
			case *types.ClusterFailoverLevelAdmissionControlPolicy:
				policy = "failover_level"
				if haAC && p != nil {
					s.Gauge(n("cluster_ha_failover_level_configured"), "Configured number of host failures to tolerate ('host failures' policy).", float64(p.FailoverLevel), l)
				}
			case *types.ClusterFailoverResourcesAdmissionControlPolicy:
				policy = "resources_percent"
				if haAC && p != nil {
					s.Gauge(n("cluster_ha_failover_cpu_resources_configured_percent"), "CPU percentage reserved for HA failover (configured or computed).", float64(p.CpuFailoverResourcesPercent), l)
					s.Gauge(n("cluster_ha_failover_memory_resources_configured_percent"), "Memory percentage reserved for HA failover (configured or computed).", float64(p.MemoryFailoverResourcesPercent), l)
				}
			case *types.ClusterFailoverHostAdmissionControlPolicy:
				policy = "failover_hosts"
			}
			s.Gauge(n("cluster_ha_admission_control_policy"), "HA admission control policy (policy label, value 1).", 1, append(append(L{}, l...), "policy", policy))
			s.Gauge(n("cluster_ha_host_monitoring_enabled"), "1 if HA host monitoring is enabled.", b2f(cfg.DasConfig.HostMonitoring == "enabled"), l)
			s.Gauge(n("cluster_drs_enabled"), "1 if DRS is enabled.", bp2f(cfg.DrsConfig.Enabled), l)
			s.Gauge(n("cluster_drs_fully_automated"), "1 if DRS is fully automated.", b2f(cfg.DrsConfig.DefaultVmBehavior == types.DrsBehaviorFullyAutomated), l)
			vsanOn := false
			if cfg.VsanConfigInfo != nil {
				vsanOn = bp2f(cfg.VsanConfigInfo.Enabled) == 1
			}
			s.Gauge(n("cluster_vsan_enabled"), "1 if vSAN is enabled on the cluster.", b2f(vsanOn), l)
			if cfg.DpmConfigInfo != nil {
				s.Gauge(n("cluster_dpm_enabled"), "1 if DPM (power management) is enabled.", bp2f(cfg.DpmConfigInfo.Enabled), l)
			}
		}
	}
	return nil
}
