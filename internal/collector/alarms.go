package collector

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// collectAlarms remonte toutes les alarmes vCenter actives.
//
// triggeredAlarmState est propagé vers les parents : celui du rootFolder
// contient donc l'ensemble des alarmes déclenchées de l'inventaire (VM, ESX,
// datastores, clusters, réseaux, vSAN...). Une seule lecture suffit, quel que
// soit le nombre d'objets, et cela marche à l'identique en 8.x et 9.x.
func collectAlarms(ctx context.Context, r *Run) error {
	var root mo.Folder
	pc := property.DefaultCollector(r.Vim)
	if err := pc.RetrieveOne(ctx, r.Vim.ServiceContent.RootFolder, []string{"triggeredAlarmState"}, &root); err != nil {
		return err
	}
	states := root.TriggeredAlarmState

	// Résolution des noms d'alarmes et d'entités en lot.
	alarmRefs := map[types.ManagedObjectReference]bool{}
	entityRefs := map[types.ManagedObjectReference]bool{}
	for _, st := range states {
		alarmRefs[st.Alarm] = true
		if _, ok := r.Topo.Name[st.Entity.Value]; !ok {
			entityRefs[st.Entity] = true
		}
	}
	alarmNames := map[string]string{}
	if len(alarmRefs) > 0 {
		var oc []types.ObjectContent
		if err := pc.Retrieve(ctx, keys(alarmRefs), []string{"info.name"}, &oc); err == nil {
			for _, o := range oc {
				for _, p := range o.PropSet {
					alarmNames[o.Obj.Value] = fmt.Sprint(p.Val)
				}
			}
		}
	}
	entityNames := map[string]string{}
	parents := map[string]types.ManagedObjectReference{}
	if len(entityRefs) > 0 {
		var oc []types.ObjectContent
		if err := pc.Retrieve(ctx, keys(entityRefs), []string{"name", "parent"}, &oc); err == nil {
			for _, o := range oc {
				for _, p := range o.PropSet {
					switch p.Name {
					case "name":
						entityNames[o.Obj.Value] = fmt.Sprint(p.Val)
					case "parent":
						if ref, ok := p.Val.(types.ManagedObjectReference); ok {
							parents[o.Obj.Value] = ref
						}
					}
				}
			}
		}
	}

	byStatus := map[string]int{"yellow": 0, "red": 0}
	for _, st := range states {
		name := alarmNames[st.Alarm.Value]
		if name == "" {
			name = st.Alarm.Value
		}
		ename := r.Topo.Name[st.Entity.Value]
		if ename == "" {
			ename = entityNames[st.Entity.Value]
		}
		dc := r.Topo.Datacenter(st.Entity.Value)
		if dc == "" {
			if p, ok := parents[st.Entity.Value]; ok {
				dc = r.Topo.Datacenter(p.Value)
			}
		}
		status := string(st.OverallStatus)
		byStatus[status]++
		ack := "false"
		if st.Acknowledged != nil && *st.Acknowledged {
			ack = "true"
		}
		l := L{
			"alarm", name, "alarm_moid", st.Alarm.Value,
			"entity", ename, "entity_type", st.Entity.Type, "entity_moid", st.Entity.Value,
			"datacenter", dc, "status", status, "acknowledged", ack,
		}
		r.Sink.Gauge(n("alarm_triggered"), "Alarme vCenter active sur une entité, valeur = sévérité"+statusHelp+".", statusValue(st.OverallStatus), l)
		r.Sink.Gauge(n("alarm_triggered_timestamp_seconds"), "Horodatage de déclenchement de l'alarme vCenter.", float64(st.Time.Unix()), l)
	}
	for s, c := range byStatus {
		r.Sink.Gauge(n("alarms_active"), "Nombre d'alarmes vCenter actives par sévérité.", float64(c), L{"status", s})
	}
	return nil
}

func keys(m map[types.ManagedObjectReference]bool) []types.ManagedObjectReference {
	out := make([]types.ManagedObjectReference, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
