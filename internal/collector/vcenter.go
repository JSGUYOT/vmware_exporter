package collector

import (
	"context"

	"github.com/vmware/govmomi/vim25/mo"

	"github.com/example/vmware-exporter/internal/vsphere"
)

func collectVCenter(ctx context.Context, r *Run) error {
	a := r.Vim.ServiceContent.About
	s := r.Sink
	s.Gauge(n("vcenter_info"), "Informations sur le vCenter (valeur toujours 1).", 1, L{
		"product", a.FullName, "version", a.Version, "build", a.Build,
		"api_version_server", a.ApiVersion, "api_version_used", r.Client.APIVersion,
		"product_line", a.ProductLineId, "instance_uuid", a.InstanceUuid,
	})
	s.Gauge(n("vcenter_version_major"), "Version majeure du vCenter (8 = vCenter 8, 9 = VCF/vSphere 9).",
		float64(vsphere.MajorVersion(a.Version)), nil)

	// Nombre d'objets par type d'après l'arborescence.
	counts := map[string]int{}
	for _, t := range r.Topo.Type {
		counts[t]++
	}
	for t, c := range counts {
		s.Gauge(n("inventory_objects"), "Nombre d'objets d'inventaire par type.", float64(c), L{"type", t})
	}

	// Sessions actives (nécessite le privilège Sessions.TerminateSession : erreur tolérée).
	if r.Vim.ServiceContent.SessionManager != nil {
		var sm mo.SessionManager
		if err := mo.RetrieveProperties(ctx, r.Vim, r.Vim.ServiceContent.PropertyCollector,
			*r.Vim.ServiceContent.SessionManager, &sm); err == nil {
			s.Gauge(n("vcenter_sessions"), "Nombre de sessions vCenter ouvertes (si le compte a le droit de les lister).", float64(len(sm.SessionList)), nil)
		}
	}

	if exp, err := vsphere.CertificateExpiry(ctx, r.Cfg); err == nil {
		s.Gauge(n("vcenter_certificate_expiry_timestamp_seconds"), "Date d'expiration du certificat TLS du vCenter (epoch).", float64(exp.Unix()), nil)
	} else {
		r.Log.Debug("lecture du certificat TLS impossible", "err", err)
	}
	return nil
}
