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
		sm := ds.Summary
		l := L{"datacenter", dc, "datastore", ds.Name, "datastore_moid", ds.Self.Value}
		s.Gauge(n("datastore_info"), "Informations sur le datastore (valeur toujours 1).", 1, append(append(L{}, l...),
			"type", sm.Type, "url", sm.Url, "datastore_cluster", pod, "shared", boolStr(sm.MultipleHostAccess != nil && *sm.MultipleHostAccess)))
		s.Gauge(n("datastore_capacity_bytes"), "Capacité du datastore.", float64(sm.Capacity), l)
		s.Gauge(n("datastore_free_bytes"), "Espace libre du datastore.", float64(sm.FreeSpace), l)
		s.Gauge(n("datastore_uncommitted_bytes"), "Espace provisionné non consommé (thin) sur le datastore.", float64(sm.Uncommitted), l)
		prov := float64(sm.Capacity - sm.FreeSpace + sm.Uncommitted)
		s.Gauge(n("datastore_provisioned_bytes"), "Espace provisionné total (utilisé + thin non consommé).", prov, l)
		s.Gauge(n("datastore_accessible"), "1 si le datastore est accessible.", b2f(sm.Accessible), l)
		mm := sm.MaintenanceMode
		s.Gauge(n("datastore_maintenance_mode"), "1 si le datastore est en mode maintenance (ou en entrée).", b2f(mm != "" && mm != "normal"), l)
		s.Gauge(n("datastore_overall_status"), "Statut global du datastore"+statusHelp+".", statusValue(ds.OverallStatus), l)
		s.Gauge(n("datastore_vms"), "Nombre de VMs ayant des fichiers sur le datastore.", float64(len(ds.Vm)), l)
		mounted, inaccessible := 0, 0
		for _, h := range ds.Host {
			mounted++
			if mi := h.MountInfo; mi.Accessible != nil && !*mi.Accessible {
				inaccessible++
			}
		}
		s.Gauge(n("datastore_hosts_mounted"), "Nombre d'ESX montant le datastore.", float64(mounted), l)
		s.Gauge(n("datastore_hosts_inaccessible"), "Nombre d'ESX pour lesquels le datastore est inaccessible (APD/PDL...).", float64(inaccessible), l)
	}

	var pods []mo.StoragePod
	if err := r.retrieve(ctx, []string{"StoragePod"}, []string{"name", "summary", "overallStatus", "childEntity"}, &pods); err != nil {
		return err
	}
	for _, p := range pods {
		l := L{"datacenter", r.Topo.Datacenter(p.Self.Value), "datastore_cluster", p.Name}
		s.Gauge(n("datastore_cluster_overall_status"), "Statut global du cluster de datastores"+statusHelp+".", statusValue(p.OverallStatus), l)
		s.Gauge(n("datastore_cluster_datastores"), "Nombre de datastores dans le cluster de datastores.", float64(len(p.ChildEntity)), l)
		if p.Summary != nil {
			s.Gauge(n("datastore_cluster_capacity_bytes"), "Capacité du cluster de datastores.", float64(p.Summary.Capacity), l)
			s.Gauge(n("datastore_cluster_free_bytes"), "Espace libre du cluster de datastores.", float64(p.Summary.FreeSpace), l)
		}
	}
	return nil
}
