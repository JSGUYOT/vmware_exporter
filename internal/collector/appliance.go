package collector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/vmware-exporter/internal/vsphere"
)

// vCenter appliance (VAMI) health endpoints exposed via /api, available on 8.x and 9.x.
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

// collectAppliance monitors vCenter itself (VAMI health, services, backups).
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
				r.Log.Debug("appliance endpoint failed", "endpoint", name, "err", err)
			}
		} else {
			okCount++
		}
		s.Gauge(n("appliance_endpoint_up"), "1 if the appliance REST endpoint responded (0 = not available on this version or insufficient privileges).", up, L{"endpoint", name})
	}

	for _, h := range applianceHealth {
		var st string
		err := rc.Get(ctx, "/api/appliance/health/"+h, &st)
		endpoint("health/"+h, err)
		if err == nil {
			s.Gauge(n("appliance_health"), "Health of a vCenter appliance component"+statusHelp+" (orange counted as 2).", healthString(st), L{"component", h})
		}
	}

	var ver struct {
		Version, Build, Product, Type string
	}
	err = rc.Get(ctx, "/api/appliance/system/version", &ver)
	endpoint("system/version", err)
	if err == nil {
		s.Gauge(n("appliance_info"), "vCenter appliance version (value always 1).", 1, L{"version", ver.Version, "build", ver.Build, "product", ver.Product, "type", ver.Type})
	}
	var uptime float64
	err = rc.Get(ctx, "/api/appliance/system/uptime", &uptime)
	endpoint("system/uptime", err)
	if err == nil {
		s.Gauge(n("appliance_uptime_seconds"), "vCenter appliance uptime.", uptime, nil)
	}

	services := map[string]vcService{}
	err = rc.Get(ctx, "/api/vcenter/services", &services)
	endpoint("vcenter/services", err)
	if err == nil {
		for name, sv := range services {
			l := L{"service", name, "startup_type", strings.ToLower(sv.StartupType)}
			s.Gauge(n("appliance_service_started"), "1 if the vCenter service is started.", b2f(strings.EqualFold(sv.State, "STARTED")), l)
			h := 0.0 // 0=unknown 1=healthy 2=warnings 3=degraded
			switch strings.ToUpper(sv.Health) {
			case "HEALTHY":
				h = 1
			case "HEALTHY_WITH_WARNINGS":
				h = 2
			case "DEGRADED":
				h = 3
			}
			s.Gauge(n("appliance_service_health"), "vCenter service health (0=unknown, 1=healthy, 2=warnings, 3=degraded).", h, l)
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
		s.Gauge(n("appliance_backup_jobs"), "Number of known file-based backup jobs.", float64(len(jobs)), nil)
		if !lastOK.IsZero() {
			s.Gauge(n("appliance_backup_last_success_timestamp_seconds"), "End time of the last successful vCenter backup.", float64(lastOK.Unix()), nil)
		}
		if lastStatus != "" {
			s.Gauge(n("appliance_backup_last_succeeded"), "1 if the last vCenter backup succeeded.", b2f(lastStatus == "SUCCEEDED"), nil)
		}
	}
	if okCount == 0 {
		return fmt.Errorf("no appliance /api endpoint reachable (%d tried): check privileges and HTTPS access", tried)
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
