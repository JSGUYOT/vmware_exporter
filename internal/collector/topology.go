package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// Topology is a lightweight view of the inventory tree (name + parent) used
// to label each object with its datacenter and cluster.
type Topology struct {
	Name   map[string]string
	Type   map[string]string
	Parent map[string]types.ManagedObjectReference
}

func newTopology() *Topology {
	return &Topology{Name: map[string]string{}, Type: map[string]string{}, Parent: map[string]types.ManagedObjectReference{}}
}

// Datacenter walks up the tree to the Datacenter.
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

// ClusterOfHost returns the cluster name of a host ("" if standalone).
func (t *Topology) ClusterOfHost(hostMoid string) string {
	p, ok := t.Parent[hostMoid]
	if ok && p.Type == "ClusterComputeResource" {
		return t.Name[p.Value]
	}
	return ""
}

// ClusterOfPool returns the cluster that owns a resource pool / vApp.
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

// FolderOf returns the name of the VM folder holding an object: "" when it is
// the datacenter's root VM folder or when the object is not in a folder (vApp).
func (t *Topology) FolderOf(parent *types.ManagedObjectReference) string {
	if parent == nil || parent.Type != "Folder" {
		return ""
	}
	if p, ok := t.Parent[parent.Value]; ok && p.Type == "Datacenter" {
		return ""
	}
	return t.Name[parent.Value]
}

// loadTopology loads name+parent of containers and hosts.
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

// retrieve queries the collection's ContainerView.
//
//   - If a property does not exist in the vCenter API version (InvalidProperty,
//     typically added or removed between 8.x and 9.x), it is dropped and the
//     request is replayed: collection degrades instead of failing.
//   - If a property is missing on a specific object (MissingSet: privileges on
//     that object, host unreachable for a config.* path...), only that field is
//     skipped for that object; other objects are unaffected.
func (r *Run) retrieve(ctx context.Context, kinds, props []string, dst any) error {
	ps := append([]string{}, props...)
	var oc []types.ObjectContent
	for attempt := 0; ; attempt++ {
		if attempt >= 8 {
			return fmt.Errorf("too many invalid properties for %v", kinds)
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
		r.Log.Warn("property not supported by this vCenter version, skipped",
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
			r.Log.Debug("property unavailable on an object, skipped",
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
		// offending property not found verbatim: drop the paths that contain it
		out = out[:0]
		for _, p := range list {
			if !strings.Contains(p, bad) {
				out = append(out, p)
			}
		}
	}
	return out
}
