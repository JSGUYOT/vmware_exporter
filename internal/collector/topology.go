package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// Topology est une vue légère de l'arborescence (nom + parent) servant à
// étiqueter chaque objet avec son datacenter et son cluster.
type Topology struct {
	Name   map[string]string
	Type   map[string]string
	Parent map[string]types.ManagedObjectReference
}

func newTopology() *Topology {
	return &Topology{Name: map[string]string{}, Type: map[string]string{}, Parent: map[string]types.ManagedObjectReference{}}
}

// Datacenter remonte l'arborescence jusqu'au Datacenter.
func (t *Topology) Datacenter(moid string) string {
	for i := 0; i < 64 && moid != ""; i++ {
		if t.Type[moid] == "Datacenter" {
			return t.Name[moid]
		}
		p, ok := t.Parent[moid]
		if !ok {
			return ""
		}
		moid = p.Value
	}
	return ""
}

// ClusterOfHost renvoie le nom du cluster d'un ESX ("" si autonome).
func (t *Topology) ClusterOfHost(hostMoid string) string {
	p, ok := t.Parent[hostMoid]
	if ok && p.Type == "ClusterComputeResource" {
		return t.Name[p.Value]
	}
	return ""
}

// ClusterOfPool renvoie le cluster propriétaire d'un resource pool / vApp.
func (t *Topology) ClusterOfPool(moid string) string {
	for i := 0; i < 64 && moid != ""; i++ {
		switch t.Type[moid] {
		case "ClusterComputeResource":
			return t.Name[moid]
		case "ComputeResource", "Datacenter":
			return ""
		}
		p, ok := t.Parent[moid]
		if !ok {
			return ""
		}
		moid = p.Value
	}
	return ""
}

// loadTopology charge nom+parent des conteneurs et ESX.
func (r *Run) loadTopology(ctx context.Context) error {
	var oc []types.ObjectContent
	kinds := []string{"Datacenter", "Folder", "ClusterComputeResource", "ComputeResource", "HostSystem", "ResourcePool", "VirtualApp", "StoragePod"}
	if err := r.retrieve(ctx, kinds, []string{"name", "parent"}, &oc); err != nil {
		return err
	}
	t := newTopology()
	for _, o := range oc {
		id := o.Obj.Value
		t.Type[id] = o.Obj.Type
		for _, p := range o.PropSet {
			switch p.Name {
			case "name":
				t.Name[id] = fmt.Sprint(p.Val)
			case "parent":
				if ref, ok := p.Val.(types.ManagedObjectReference); ok {
					t.Parent[id] = ref
				}
			}
		}
	}
	r.Topo = t
	return nil
}

// retrieve interroge le ContainerView de la collecte.
//
//   - Si une propriété n'existe pas dans la version d'API du vCenter
//     (InvalidProperty, typiquement ajoutée ou retirée entre 8.x et 9.x), elle est
//     retirée et la requête rejouée : la collecte se dégrade au lieu d'échouer.
//   - Si une propriété est manquante sur un objet précis (MissingSet : droits sur
//     cet objet, ESX injoignable pour un chemin config.*...), seul ce champ est
//     ignoré pour cet objet ; les autres objets ne sont pas impactés.
func (r *Run) retrieve(ctx context.Context, kinds, props []string, dst any) error {
	ps := append([]string{}, props...)
	var oc []types.ObjectContent
	for attempt := 0; ; attempt++ {
		if attempt >= 8 {
			return fmt.Errorf("trop de propriétés invalides pour %v", kinds)
		}
		oc = nil
		err := r.View.Retrieve(ctx, kinds, ps, &oc)
		if err == nil {
			break
		}
		bad := invalidProperty(err)
		if bad == "" {
			return err
		}
		r.Log.Warn("propriété non supportée par cette version de vCenter, ignorée",
			"property", bad, "kinds", strings.Join(kinds, ","), "api", r.Client.APIVersion)
		r.unsupported.Add(1)
		ps = remove(ps, bad)
		if len(ps) == 0 {
			return err
		}
	}
	for i := range oc {
		if len(oc[i].MissingSet) == 0 {
			continue
		}
		for _, m := range oc[i].MissingSet {
			fault := ""
			if m.Fault.Fault != nil {
				fault = fmt.Sprintf("%T", m.Fault.Fault)
			}
			r.Log.Debug("propriété indisponible sur un objet, ignorée",
				"object", oc[i].Obj.Value, "property", m.Path, "fault", fault)
		}
		r.missing.Add(int64(len(oc[i].MissingSet)))
		oc[i].MissingSet = nil
	}
	if d, ok := dst.(*[]types.ObjectContent); ok {
		*d = oc
		return nil
	}
	return mo.LoadObjectContent(oc, dst)
}

func invalidProperty(err error) string {
	if !soap.IsSoapFault(err) {
		return ""
	}
	switch f := soap.ToSoapFault(err).VimFault().(type) {
	case types.InvalidProperty:
		return f.Name
	case *types.InvalidProperty:
		return f.Name
	}
	return ""
}

func remove(list []string, bad string) []string {
	out := list[:0:0]
	for _, p := range list {
		if p != bad && !strings.HasPrefix(p, bad+".") {
			out = append(out, p)
		}
	}
	if len(out) == len(list) {
		// propriété fautive non trouvée telle quelle : retire les chemins qui la contiennent
		out = out[:0]
		for _, p := range list {
			if !strings.Contains(p, bad) {
				out = append(out, p)
			}
		}
	}
	return out
}
