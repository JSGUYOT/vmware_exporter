package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"
)

func collectNetwork(ctx context.Context, r *Run) error {
	s := r.Sink
	var dvss []mo.DistributedVirtualSwitch
	if err := r.retrieve(ctx, []string{"DistributedVirtualSwitch"},
		[]string{"name", "parent", "overallStatus", "summary.numPorts", "summary.productInfo", "summary.hostMember", "runtime.hostMemberRuntime"}, &dvss); err != nil {
		return err
	}
	for _, d := range dvss {
		dc := ""
		if d.Parent != nil {
			dc = r.Topo.Datacenter(d.Parent.Value)
		}
		ver := ""
		if d.Summary.ProductInfo != nil {
			ver = d.Summary.ProductInfo.Version
		}
		l := L{"datacenter", dc, "dvs", d.Name}
		s.Gauge(n("dvs_info"), "Informations sur le vDS (valeur toujours 1).", 1, append(append(L{}, l...), "version", ver))
		s.Gauge(n("dvs_overall_status"), "Statut global du vDS"+statusHelp+".", statusValue(d.OverallStatus), l)
		s.Gauge(n("dvs_ports"), "Nombre de ports du vDS.", float64(d.Summary.NumPorts), l)
		s.Gauge(n("dvs_hosts"), "Nombre d'ESX membres du vDS.", float64(len(d.Summary.HostMember)), l)
		if d.Runtime != nil {
			byStatus := map[string]int{"up": 0, "pending": 0, "outOfSync": 0, "warning": 0, "disconnected": 0, "down": 0}
			for _, hm := range d.Runtime.HostMemberRuntime {
				byStatus[hm.Status]++
				hl := append(append(L{}, l...), "host", r.Topo.Name[hm.Host.Value])
				s.Gauge(n("dvs_host_member_up"), "1 si l'ESX membre du vDS est 'up' (sinon outOfSync, warning, down...).", b2f(hm.Status == "up"), hl)
			}
			for st, c := range byStatus {
				s.Gauge(n("dvs_host_members"), "Nombre d'ESX membres du vDS par statut.", float64(c), append(append(L{}, l...), "status", st))
			}
		}
	}

	var nets []mo.Network
	if err := r.retrieve(ctx, []string{"Network"}, []string{"name", "parent", "overallStatus", "summary", "host", "vm"}, &nets); err != nil {
		return err
	}
	for _, nw := range nets {
		dc := ""
		if nw.Parent != nil {
			dc = r.Topo.Datacenter(nw.Parent.Value)
		}
		kind := "standard"
		switch nw.Self.Type {
		case "DistributedVirtualPortgroup":
			kind = "distributed"
		case "OpaqueNetwork":
			kind = "opaque"
		}
		acc := true
		if nw.Summary != nil {
			if ns := nw.Summary.GetNetworkSummary(); ns != nil {
				acc = ns.Accessible
			}
		}
		l := L{"datacenter", dc, "network", nw.Name, "network_type", kind, "network_moid", nw.Self.Value}
		s.Gauge(n("network_overall_status"), "Statut global du réseau / portgroup"+statusHelp+".", statusValue(nw.OverallStatus), l)
		s.Gauge(n("network_accessible"), "1 si le réseau est accessible depuis au moins un ESX.", b2f(acc), l)
		s.Gauge(n("network_vms"), "Nombre de VMs connectées au réseau.", float64(len(nw.Vm)), l)
		s.Gauge(n("network_hosts"), "Nombre d'ESX ayant accès au réseau.", float64(len(nw.Host)), l)
	}
	return nil
}
