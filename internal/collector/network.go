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
		l := L{"dc_name", dc, "dvs", d.Name}
		s.Gauge(n("dvs_info"), "vDS information (value always 1).", 1, append(append(L{}, l...), "version", ver))
		s.Gauge(n("dvs_overall_status"), "Overall status of the vDS"+statusHelp+".", statusValue(d.OverallStatus), l)
		s.Gauge(n("dvs_ports"), "Number of ports on the vDS.", float64(d.Summary.NumPorts), l)
		s.Gauge(n("dvs_hosts"), "Number of hosts that are members of the vDS.", float64(len(d.Summary.HostMember)), l)
		if d.Runtime != nil {
			byStatus := map[string]int{"up": 0, "pending": 0, "outOfSync": 0, "warning": 0, "disconnected": 0, "down": 0}
			for _, hm := range d.Runtime.HostMemberRuntime {
				byStatus[hm.Status]++
				hl := append(append(L{}, l...), "host_name", r.Topo.Name[hm.Host.Value])
				s.Gauge(n("dvs_host_member_up"), "1 if the vDS member host is 'up' (otherwise outOfSync, warning, down...).", b2f(hm.Status == "up"), hl)
			}
			for st, c := range byStatus {
				s.Gauge(n("dvs_host_members"), "Number of vDS member hosts by status.", float64(c), append(append(L{}, l...), "status", st))
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
		l := L{"dc_name", dc, "network", nw.Name, "network_type", kind, "network_moid", nw.Self.Value}
		s.Gauge(n("network_overall_status"), "Overall status of the network / portgroup"+statusHelp+".", statusValue(nw.OverallStatus), l)
		s.Gauge(n("network_accessible"), "1 if the network is accessible from at least one host.", b2f(acc), l)
		s.Gauge(n("network_vms"), "Number of VMs connected to the network.", float64(len(nw.Vm)), l)
		s.Gauge(n("network_hosts"), "Number of hosts with access to the network.", float64(len(nw.Host)), l)
	}
	return nil
}
