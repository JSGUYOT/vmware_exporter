package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"

	"github.com/JSGUYOT/vmware_exporter/internal/vsphere"
)

func collectVCenter(ctx context.Context, r *Run) error {
	a := r.Vim.ServiceContent.About
	s := r.Sink
	s.Gauge(n("vcenter_info"), "vCenter information (value always 1).", 1, L{
		"product", a.FullName, "version", a.Version, "build", a.Build,
		"api_version_server", a.ApiVersion, "api_version_used", r.Client.APIVersion,
		"product_line", a.ProductLineId, "instance_uuid", a.InstanceUuid,
	})
	s.Gauge(n("vcenter_version_major"), "vCenter major version (8 = vCenter 8, 9 = VCF/vSphere 9).",
		float64(vsphere.MajorVersion(a.Version)), nil)

	// Object count per type, from the inventory tree.
	counts := map[string]int{}
	for _, t := range r.Topo.Type {
		counts[t]++
	}
	for t, c := range counts {
		s.Gauge(n("inventory_objects"), "Number of inventory objects by type.", float64(c), L{"type", t})
	}

	// Active sessions (requires the Sessions.TerminateSession privilege: errors are tolerated).
	if r.Vim.ServiceContent.SessionManager != nil {
		var sm mo.SessionManager
		if err := mo.RetrieveProperties(ctx, r.Vim, r.Vim.ServiceContent.PropertyCollector,
			*r.Vim.ServiceContent.SessionManager, &sm); err == nil {
			s.Gauge(n("vcenter_sessions"), "Number of open vCenter sessions (if the account is allowed to list them).", float64(len(sm.SessionList)), nil)
		}
	}

	if exp, err := vsphere.CertificateExpiry(ctx, r.Cfg); err == nil {
		s.Gauge(n("vcenter_certificate_expiry_timestamp_seconds"), "vCenter TLS certificate expiry date (epoch).", float64(exp.Unix()), nil)
	} else {
		r.Log.Debug("unable to read TLS certificate", "err", err)
	}
	return nil
}
