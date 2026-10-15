package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

func collectResourcePools(ctx context.Context, r *Run) error {
	var pools []mo.ResourcePool
	if err := r.retrieve(ctx, []string{"ResourcePool"},
		[]string{"name", "overallStatus", "summary", "config.cpuAllocation", "config.memoryAllocation", "vm"}, &pools); err != nil {
		return err
	}
	s := r.Sink
	for _, p := range pools {
		l := L{"datacenter", r.Topo.Datacenter(p.Self.Value), "cluster", r.Topo.ClusterOfPool(p.Self.Value),
			"resource_pool", p.Name, "resource_pool_moid", p.Self.Value}
		s.Gauge(n("resourcepool_overall_status"), "Statut global du resource pool"+statusHelp+".", statusValue(p.OverallStatus), l)
		s.Gauge(n("resourcepool_vms"), "Nombre de VMs directement dans le resource pool.", float64(len(p.Vm)), l)
		if sum, ok := p.Summary.(*types.ResourcePoolSummary); ok && sum != nil && sum.QuickStats != nil {
			q := sum.QuickStats
			s.Gauge(n("resourcepool_cpu_usage_mhz"), "CPU consommé par le resource pool (MHz).", float64(q.OverallCpuUsage), l)
			s.Gauge(n("resourcepool_memory_guest_active_bytes"), "Mémoire active des invités du pool.", float64(q.GuestMemoryUsage)*1024*1024, l)
			s.Gauge(n("resourcepool_memory_host_consumed_bytes"), "Mémoire consommée sur les ESX par le pool.", float64(q.HostMemoryUsage)*1024*1024, l)
			s.Gauge(n("resourcepool_memory_ballooned_bytes"), "Mémoire en ballooning dans le pool.", float64(q.BalloonedMemory)*1024*1024, l)
			s.Gauge(n("resourcepool_memory_swapped_bytes"), "Mémoire swappée dans le pool.", float64(q.SwappedMemory)*1024*1024, l)
		}
		if p.Config.CpuAllocation.Limit != nil {
			s.Gauge(n("resourcepool_cpu_limit_mhz"), "Limite CPU du pool (MHz, -1 = illimité).", float64(*p.Config.CpuAllocation.Limit), l)
		}
		if p.Config.CpuAllocation.Reservation != nil {
			s.Gauge(n("resourcepool_cpu_reservation_mhz"), "Réservation CPU du pool (MHz).", float64(*p.Config.CpuAllocation.Reservation), l)
		}
		if p.Config.MemoryAllocation.Limit != nil {
			lim := float64(*p.Config.MemoryAllocation.Limit)
			if lim > 0 {
				lim *= 1024 * 1024
			}
			s.Gauge(n("resourcepool_memory_limit_bytes"), "Limite mémoire du pool (-1 = illimité).", lim, l)
		}
		if p.Config.MemoryAllocation.Reservation != nil {
			s.Gauge(n("resourcepool_memory_reservation_bytes"), "Réservation mémoire du pool.", float64(*p.Config.MemoryAllocation.Reservation)*1024*1024, l)
		}
	}
	return nil
}
