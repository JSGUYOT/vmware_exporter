package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/vmware-exporter/internal/vsphere"
)

// Endpoints de santé de l'appliance vCenter (VAMI) exposés via /api, présents en 8.x et 9.x.
var applianceHealth = []string{"system", "load", "mem", "storage", "swap", "database", "database-storage", "applmgmt", "software-packages"}

type vcService struct {
	State       string `json:"state"`
	Health      string `json:"health"`
	StartupType string `json:"startup_type"`
}

type backupJob struct {
	Status    string `json:"status"`
	State     string `json:"state"`
	EndTime   string `json:"end_time"`
	StartTime string `json:"start_time"`
	Type      string `json:"type"`
}

// collectAppliance supervise le vCenter lui-même (santé VAMI, services, sauvegardes).
func collectAppliance(ctx context.Context, r *Run) error {
	if !r.Vim.IsVC() {
		return nil
	}
	rc, err := r.Client.REST(ctx)
	if err != nil {
		return err
	}
	s := r.Sink
	okCount, tried := 0, 0
	endpoint := func(name string, err error) {
		tried++
		up := 1.0
		if err != nil {
			up = 0
			if !errors.Is(err, vsphere.ErrNotFound) {
				r.Log.Debug("endpoint appliance en échec", "endpoint", name, "err", err)
			}
		} else {
			okCount++
		}
		s.Gauge(n("appliance_endpoint_up"), "1 si l'endpoint REST de l'appliance a répondu (0 = absent sur cette version ou droits insuffisants).", up, L{"endpoint", name})
	}

	for _, h := range applianceHealth {
		var st string
		err := rc.Get(ctx, "/api/appliance/health/"+h, &st)
		endpoint("health/"+h, err)
		if err == nil {
			s.Gauge(n("appliance_health"), "Santé d'un composant de l'appliance vCenter"+statusHelp+" (orange compté comme 2).", healthString(st), L{"component", h})
		}
	}

	var ver struct {
		Version, Build, Product, Type string
	}
	err = rc.Get(ctx, "/api/appliance/system/version", &ver)
	endpoint("system/version", err)
	if err == nil {
		s.Gauge(n("appliance_info"), "Version de l'appliance vCenter (valeur toujours 1).", 1, L{"version", ver.Version, "build", ver.Build, "product", ver.Product, "type", ver.Type})
	}
	var uptime float64
	err = rc.Get(ctx, "/api/appliance/system/uptime", &uptime)
	endpoint("system/uptime", err)
	if err == nil {
		s.Gauge(n("appliance_uptime_seconds"), "Uptime de l'appliance vCenter.", uptime, nil)
	}

	services := map[string]vcService{}
	err = rc.Get(ctx, "/api/vcenter/services", &services)
	endpoint("vcenter/services", err)
	if err == nil {
		for name, sv := range services {
			l := L{"service", name, "startup_type", strings.ToLower(sv.StartupType)}
			s.Gauge(n("appliance_service_started"), "1 si le service vCenter est démarré.", b2f(strings.EqualFold(sv.State, "STARTED")), l)
			h := 0.0 // 0=inconnu 1=healthy 2=warnings 3=degraded
			switch strings.ToUpper(sv.Health) {
			case "HEALTHY":
				h = 1
			case "HEALTHY_WITH_WARNINGS":
				h = 2
			case "DEGRADED":
				h = 3
			}
			s.Gauge(n("appliance_service_health"), "Santé du service vCenter (0=inconnu, 1=healthy, 2=warnings, 3=degraded).", h, l)
		}
	}

	jobs := map[string]backupJob{}
	err = rc.Get(ctx, "/api/appliance/recovery/backup/job/details", &jobs)
	endpoint("recovery/backup/job/details", err)
	if err == nil {
		var lastEnd, lastOK time.Time
		lastStatus := ""
		for _, j := range jobs {
			st := strings.ToUpper(firstNonEmpty(j.Status, j.State))
			end, perr := time.Parse(time.RFC3339, j.EndTime)
			if perr != nil {
				continue
			}
			if end.After(lastEnd) {
				lastEnd, lastStatus = end, st
			}
			if st == "SUCCEEDED" && end.After(lastOK) {
				lastOK = end
			}
		}
		s.Gauge(n("appliance_backup_jobs"), "Nombre de jobs de sauvegarde file-based connus.", float64(len(jobs)), nil)
		if !lastOK.IsZero() {
			s.Gauge(n("appliance_backup_last_success_timestamp_seconds"), "Fin de la dernière sauvegarde réussie du vCenter.", float64(lastOK.Unix()), nil)
		}
		if lastStatus != "" {
			s.Gauge(n("appliance_backup_last_succeeded"), "1 si la dernière sauvegarde du vCenter a réussi.", b2f(lastStatus == "SUCCEEDED"), nil)
		}
	}
	if okCount == 0 {
		return fmt.Errorf("aucun endpoint /api de l'appliance accessible (%d essayés) : vérifier droits et accès HTTPS", tried)
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
