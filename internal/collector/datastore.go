package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"
)

func collectDatastores(ctx context.Context, r *Run) error {
	var dss []mo.Datastore
	if err := r.retrieve(ctx, []string{"Datastore"}, []string{"name", "parent", "summary", "overallStatus", "host", "vm"}, &dss); err != nil {
		return err
	}
	s := r.Sink
	emit, pry := *r.Cfg.Collectors.Datastore, *r.Cfg.Collectors.Pryorda
	for i := range dss {
		ds := &dss[i]
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
		if pry {
			pryordaDatastore(r, ds, dc, pod)
		}
		if !emit {
			continue
		}
		sm := ds.Summary
		l := L{"datacenter", dc, "datastore", ds.Name, "datastore_moid", ds.Self.Value}
		s.Gauge(n("datastore_info"), "Datastore information (value always 1).", 1, append(append(L{}, l...),
			"type", sm.Type, "url", sm.Url, "datastore_cluster", pod, "shared", boolStr(sm.MultipleHostAccess != nil && *sm.MultipleHostAccess)))
		s.Gauge(n("datastore_capacity_bytes"), "Datastore capacity.", float64(sm.Capacity), l)
		s.Gauge(n("datastore_free_bytes"), "Datastore free space.", float64(sm.FreeSpace), l)
		s.Gauge(n("datastore_uncommitted_bytes"), "Provisioned but unconsumed (thin) space on the datastore.", float64(sm.Uncommitted), l)
		prov := float64(sm.Capacity - sm.FreeSpace + sm.Uncommitted)
		s.Gauge(n("datastore_provisioned_bytes"), "Total provisioned space (used + unconsumed thin).", prov, l)
		s.Gauge(n("datastore_is_accessible"), "1 if the datastore is accessible.", b2f(sm.Accessible), l)
		mm := sm.MaintenanceMode
		s.Gauge(n("datastore_in_maintenance"), "1 if the datastore is in (or entering) maintenance mode.", b2f(mm != "" && mm != "normal"), l)
		s.Gauge(n("datastore_overall_status"), "Overall status of the datastore"+statusHelp+".", statusValue(ds.OverallStatus), l)
		s.Gauge(n("datastore_vms_hosted"), "Number of VMs with files on the datastore.", float64(len(ds.Vm)), l)
		mounted, inaccessible := 0, 0
		for _, h := range ds.Host {
			mounted++
			if mi := h.MountInfo; mi.Accessible != nil && !*mi.Accessible {
				inaccessible++
			}
		}
		s.Gauge(n("datastore_hosts_mounted"), "Number of hosts mounting the datastore.", float64(mounted), l)
		s.Gauge(n("datastore_hosts_inaccessible"), "Number of hosts on which the datastore is inaccessible (APD/PDL...).", float64(inaccessible), l)
	}

	if !emit {
		return nil
	}
	var pods []mo.StoragePod
	if err := r.retrieve(ctx, []string{"StoragePod"}, []string{"name", "summary", "overallStatus", "childEntity"}, &pods); err != nil {
		return err
	}
	for _, p := range pods {
		l := L{"datacenter", r.Topo.Datacenter(p.Self.Value), "datastore_cluster", p.Name}
		s.Gauge(n("datastore_cluster_overall_status"), "Overall status of the datastore cluster"+statusHelp+".", statusValue(p.OverallStatus), l)
		s.Gauge(n("datastore_cluster_datastores"), "Number of datastores in the datastore cluster.", float64(len(p.ChildEntity)), l)
		if p.Summary != nil {
			s.Gauge(n("datastore_cluster_capacity_bytes"), "Datastore cluster capacity.", float64(p.Summary.Capacity), l)
			s.Gauge(n("datastore_cluster_free_bytes"), "Datastore cluster free space.", float64(p.Summary.FreeSpace), l)
		}
	}
	return nil
}
