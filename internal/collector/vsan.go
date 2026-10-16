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

// collectVSAN reads the vSAN health summary (cached health service results,
// so tests are not re-run) for each vSAN cluster.
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
		// Without a version the SOAPAction would be "urn:vsan/" (rejected): fall back to the negotiated vim25 version.
		vc.Client.Version = r.Vim.Client.Version
		r.Log.Warn("vSAN endpoint version not negotiated, falling back to vim25 version", "err", err, "version", vc.Client.Version)
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
			r.Sink.Gauge(n("vsan_health_query_success"), "1 if the vSAN health summary could be read for the cluster.", 0, l)
			continue
		}
		r.Sink.Gauge(n("vsan_health_query_success"), "1 if the vSAN health summary could be read for the cluster.", 1, l)
		sum := res.Returnval
		r.Sink.Gauge(n("vsan_cluster_health"), "Overall vSAN health of the cluster"+statusHelp+".", healthString(sum.OverallHealth), l)
		if sum.Timestamp != nil {
			r.Sink.Gauge(n("vsan_health_timestamp_seconds"), "Time of the last vSAN health check.", float64(sum.Timestamp.Unix()), l)
		}
		for _, g := range sum.Groups {
			gl := append(append(L{}, l...), "group", g.GroupName)
			r.Sink.Gauge(n("vsan_health_group"), "Health of a vSAN test group"+statusHelp+".", healthString(g.GroupHealth), gl)
			for _, t := range g.GroupTests {
				tl := append(append(L{}, gl...), "test", t.TestName)
				r.Sink.Gauge(n("vsan_health_test"), "Result of a vSAN health test"+statusHelp+".", healthString(t.TestHealth), tl)
			}
		}
	}
	return firstErr
}
