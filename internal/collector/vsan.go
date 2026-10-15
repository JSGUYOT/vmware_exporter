package collector

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vsan"
	"github.com/vmware/govmomi/vsan/methods"
	vsantypes "github.com/vmware/govmomi/vsan/types"
)

var vsanHealthSystem = vimtypes.ManagedObjectReference{Type: "VsanVcClusterHealthSystem", Value: "vsan-cluster-health-system"}

// collectVSAN lit le résumé de santé vSAN (résultats en cache du health service,
// donc sans relancer les tests) pour chaque cluster vSAN.
func collectVSAN(ctx context.Context, r *Run) error {
	if !r.Vim.IsVC() {
		return nil
	}
	var cls []mo.ClusterComputeResource
	if err := r.retrieve(ctx, []string{"ClusterComputeResource"}, []string{"name", "configurationEx"}, &cls); err != nil {
		return err
	}
	var targets []mo.ClusterComputeResource
	for _, c := range cls {
		if cfg, ok := c.ConfigurationEx.(*vimtypes.ClusterConfigInfoEx); ok && cfg.VsanConfigInfo != nil && bp2f(cfg.VsanConfigInfo.Enabled) == 1 {
			targets = append(targets, c)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	vc, err := vsan.NewClient(ctx, r.Vim)
	if err != nil {
		return err
	}
	if err := vc.Client.UseServiceVersion(vsan.Namespace); err != nil || vc.Client.Version == "" {
		// Sans version, le SOAPAction serait "urn:vsan/" (refusé) : repli sur la version vim25 négociée.
		vc.Client.Version = r.Vim.Client.Version
		r.Log.Warn("version du endpoint vSAN non négociée, repli sur la version vim25", "err", err, "version", vc.Client.Version)
	}
	var firstErr error
	for _, c := range targets {
		ref := c.Self
		l := L{"datacenter", r.Topo.Datacenter(ref.Value), "cluster", c.Name}
		res, err := methods.VsanQueryVcClusterHealthSummary(ctx, vc, &vsantypes.VsanQueryVcClusterHealthSummary{
			This: vsanHealthSystem, Cluster: &ref,
			Fields:         []string{"timestamp", "overallHealth", "overallHealthDescription", "groups"},
			FetchFromCache: vimtypes.NewBool(true), IncludeObjUuids: vimtypes.NewBool(false),
		})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("cluster %s: %w", c.Name, err)
			}
			r.Sink.Gauge(n("vsan_health_query_success"), "1 si le résumé de santé vSAN a pu être lu pour le cluster.", 0, l)
			continue
		}
		r.Sink.Gauge(n("vsan_health_query_success"), "1 si le résumé de santé vSAN a pu être lu pour le cluster.", 1, l)
		sum := res.Returnval
		r.Sink.Gauge(n("vsan_cluster_health"), "Santé globale vSAN du cluster"+statusHelp+".", healthString(sum.OverallHealth), l)
		if sum.Timestamp != nil {
			r.Sink.Gauge(n("vsan_health_timestamp_seconds"), "Horodatage du dernier test de santé vSAN.", float64(sum.Timestamp.Unix()), l)
		}
		for _, g := range sum.Groups {
			gl := append(append(L{}, l...), "group", g.GroupName)
			r.Sink.Gauge(n("vsan_health_group"), "Santé d'un groupe de tests vSAN"+statusHelp+".", healthString(g.GroupHealth), gl)
			for _, t := range g.GroupTests {
				tl := append(append(L{}, gl...), "test", t.TestName)
				r.Sink.Gauge(n("vsan_health_test"), "Résultat d'un test de santé vSAN"+statusHelp+".", healthString(t.TestHealth), tl)
			}
		}
	}
	return firstErr
}
