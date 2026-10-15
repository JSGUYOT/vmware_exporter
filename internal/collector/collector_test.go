package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/event"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/vpx"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/example/vmware-exporter/internal/config"
	"github.com/example/vmware-exporter/internal/vsphere"
)

func testLogger() *slog.Logger {
	lv := slog.LevelWarn
	if os.Getenv("TEST_DEBUG") != "" {
		lv = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}

// newModel crée un vCenter simulé annonçant la version donnée (8.0.3 ou 9.0.0).
func newModel(t *testing.T, version, apiVersion string) (*simulator.Model, *simulator.Server) {
	t.Helper()
	m := simulator.VPX()
	m.Datacenter = 2
	m.Cluster = 2
	m.ClusterHost = 3
	m.Host = 1
	m.Machine = 3
	m.Portgroup = 2
	m.Pod = 1
	sc := vpx.ServiceContent
	about := sc.About
	about.Version = version
	about.ApiVersion = apiVersion
	about.FullName = "VMware vCenter Server " + version + " build-sim"
	about.Build = "sim"
	sc.About = about
	m.ServiceContent = sc
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	m.Service.TLS = new(tls.Config)
	s := m.Service.NewServer()
	t.Cleanup(func() { s.Close(); m.Remove() })
	return m, s
}

func targetFor(t *testing.T, s *simulator.Server, mutate func(*config.Target)) *config.Target {
	t.Helper()
	pass, _ := s.URL.User.Password()
	yml := `
targets:
  - name: vc-test
    url: ` + s.URL.Host + `
    username: ` + s.URL.User.Username() + `
    password: ` + pass + `
    insecure_skip_verify: true
    interval: 20s
    timeout: 15s
    extra_labels: {site: lab}
`
	cfg, err := config.Parse([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	tg := &cfg.Targets[0]
	if mutate != nil {
		mutate(tg)
	}
	return tg
}

type gathered struct {
	fams map[string]*dto.MetricFamily
}

func gather(t *testing.T, tg *Target) gathered {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(&Exporter{Targets: []*Target{tg}})
	fams, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	g := gathered{fams: map[string]*dto.MetricFamily{}}
	for _, f := range fams {
		g.fams[f.GetName()] = f
	}
	return g
}

func (g gathered) find(name string, labels map[string]string) []*dto.Metric {
	f, ok := g.fams[name]
	if !ok {
		return nil
	}
	var out []*dto.Metric
next:
	for _, m := range f.Metric {
		for k, v := range labels {
			found := false
			for _, lp := range m.Label {
				if lp.GetName() == k && lp.GetValue() == v {
					found = true
				}
			}
			if !found {
				continue next
			}
		}
		out = append(out, m)
	}
	return out
}

func value(m *dto.Metric) float64 {
	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	return m.Counter.GetValue()
}

func label(m *dto.Metric, name string) string {
	for _, lp := range m.Label {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func triggerVMAlarm(t *testing.T, s *simulator.Server) {
	t.Helper()
	ctx := context.Background()
	gc, err := govmomi.NewClient(ctx, s.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	c := gc.Client
	vm, err := find.NewFinder(c).VirtualMachine(ctx, "/DC0/vm/DC0_C0_RP0_VM0")
	if err != nil {
		t.Fatal(err)
	}
	ref := vm.Reference()
	err = event.NewManager(c).PostEvent(ctx, &types.EventEx{
		EventTypeId: "vcsim.vm.fatal", ObjectType: "VirtualMachine", ObjectId: ref.Value,
		Event: types.Event{Vm: &types.VmEventArgument{Vm: ref}, CreatedTime: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCompatibilityMatrix exécute une collecte complète contre un vCenter 8 et un vCenter 9 simulés.
func TestCompatibilityMatrix(t *testing.T) {
	cases := []struct{ name, version, api string }{
		{"vCenter-8.0U3", "8.0.3", "8.0.3.0"},
		{"VCF-9.0", "9.0.0", "9.0.0.0"},
		{"VCF-9.1", "9.1.0", "9.1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, s := newModel(t, tc.version, tc.api)
			triggerVMAlarm(t, s)
			cfg := targetFor(t, s, func(c *config.Target) { c.Collectors.Appliance = boolp(false) })
			tg := NewTarget(cfg, testLogger())
			tg.collectOnce(context.Background())
			g := gather(t, tg)

			if v := g.find("vmware_up", nil); len(v) != 1 || value(v[0]) != 1 {
				t.Fatalf("vmware_up attendu à 1")
			}
			info := g.find("vmware_vcenter_info", nil)
			if len(info) != 1 {
				t.Fatalf("vmware_vcenter_info absent")
			}
			if got := label(info[0], "api_version_used"); got != tc.api {
				t.Errorf("négociation: api_version_used=%q, attendu %q", got, tc.api)
			}
			if got := label(info[0], "site"); got != "lab" {
				t.Errorf("extra_labels non appliqué: %q", got)
			}
			for _, m := range g.find("vmware_collector_success", nil) {
				if value(m) != 1 {
					t.Errorf("collecteur %s en échec", label(m, "collector"))
				}
			}
			if u := g.find("vmware_exporter_unsupported_properties", nil); len(u) == 1 && value(u[0]) != 0 {
				t.Errorf("%v propriétés rejetées par le simulateur", value(u[0]))
			}
			al := g.find("vmware_alarm_triggered", map[string]string{"alarm": "vcsim VM Alarm", "entity": "DC0_C0_RP0_VM0", "status": "red"})
			if len(al) != 1 || value(al[0]) != 3 {
				t.Errorf("alarme vCenter rouge non remontée (%d séries)", len(al))
			} else if label(al[0], "datacenter") != "DC0" {
				t.Errorf("datacenter de l'alarme = %q", label(al[0], "datacenter"))
			}
			mustHave := []string{
				"vmware_cluster_overall_status", "vmware_cluster_cpu_total_mhz", "vmware_cluster_ha_enabled",
				"vmware_host_connection_state", "vmware_host_info", "vmware_host_memory_usage_bytes",
				"vmware_vm_power_state", "vmware_vm_info", "vmware_vm_snapshots", "vmware_vm_storage_committed_bytes",
				"vmware_datastore_capacity_bytes", "vmware_datastore_accessible", "vmware_datastore_cluster_overall_status",
				"vmware_dvs_overall_status", "vmware_network_overall_status", "vmware_resourcepool_overall_status",
				"vmware_datacenter_overall_status", "vmware_inventory_objects",
				"vmware_host_perf_cpu_usage_average_percent", "vmware_vm_perf_cpu_ready_summation_seconds",
			}
			for _, name := range mustHave {
				if _, ok := g.fams[name]; !ok {
					t.Errorf("métrique %s absente", name)
				}
			}
			vm := g.find("vmware_vm_power_state", map[string]string{"vm": "DC0_C0_RP0_VM0"})
			if len(vm) != 1 || label(vm[0], "datacenter") != "DC0" || label(vm[0], "host") != "" || label(vm[0], "cluster") != "" {
				t.Errorf("labels VM attendus stables (sans host/cluster): %v", vm)
			}
			vi := g.find("vmware_vm_info", map[string]string{"vm": "DC0_C0_RP0_VM0"})
			if len(vi) != 1 || label(vi[0], "cluster") != "DC0_C0" || label(vi[0], "host") == "" {
				t.Errorf("vmware_vm_info doit porter cluster/host: %v", vi)
			}
			h := g.find("vmware_host_connection_state", map[string]string{"cluster": ""})
			if len(h) == 0 {
				t.Errorf("ESX autonome (sans cluster) non remonté")
			}
		})
	}
}

// TestLint vérifie les conventions de nommage Prometheus.
func TestLint(t *testing.T) {
	_, s := newModel(t, "9.0.0", "9.0.0.0")
	cfg := targetFor(t, s, func(c *config.Target) { c.Collectors.Appliance = boolp(false) })
	tg := NewTarget(cfg, testLogger())
	tg.collectOnce(context.Background())
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(&Exporter{Targets: []*Target{tg}})
	probs, err := testutil.GatherAndLint(reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range probs {
		t.Errorf("lint %s: %s", p.Metric, p.Text)
	}
}

// TestInvalidPropertyFallback vérifie l'analyse d'un fault InvalidProperty
// (renvoyé par un vrai vCenter quand une propriété n'existe pas dans sa version d'API).
func TestInvalidPropertyFallback(t *testing.T) {
	f := &soap.Fault{Code: "ServerFaultCode", String: "InvalidProperty"}
	f.Detail.Fault = types.InvalidProperty{Name: "summary.newIn91"}
	err := soap.WrapSoapFault(f)
	if got := invalidProperty(err); got != "summary.newIn91" {
		t.Fatalf("invalidProperty=%q", got)
	}
	got := remove([]string{"name", "summary.newIn91", "summary.quickStats"}, "summary.newIn91")
	if strings.Join(got, ",") != "name,summary.quickStats" {
		t.Fatalf("remove=%v", got)
	}
	got = remove([]string{"name", "config.network", "config.network.pnic"}, "config.network")
	if strings.Join(got, ",") != "name" {
		t.Fatalf("remove sous-chemins=%v", got)
	}
}

// TestSessionRecovery vérifie la reconnexion automatique après expiration de session.
func TestSessionRecovery(t *testing.T) {
	_, s := newModel(t, "9.0.0", "9.0.0.0")
	cfg := targetFor(t, s, func(c *config.Target) { c.Collectors.Appliance = boolp(false) })
	tg := NewTarget(cfg, testLogger())
	ctx := context.Background()
	tg.collectOnce(ctx)
	vc, _ := tg.client.Get(ctx)
	// Simule un redémarrage vpxd : la session courante est détruite côté serveur.
	if err := (&sessionKiller{}).kill(ctx, vc); err != nil {
		t.Fatal(err)
	}
	tg.collectOnce(ctx)
	g := gather(t, tg)
	if v := g.find("vmware_up", nil); len(v) != 1 || value(v[0]) != 1 {
		t.Fatalf("pas de reconnexion après expiration de session")
	}
}

// TestUnreachable vérifie qu'un vCenter injoignable donne vmware_up=0 sans planter.
func TestUnreachable(t *testing.T) {
	yml := `
targets:
  - name: vc-down
    url: 127.0.0.1:1
    username: svc
    password: x
    interval: 20s
    timeout: 3s
`
	cfg, err := config.Parse([]byte(yml))
	if err != nil {
		t.Fatal(err)
	}
	tg := NewTarget(&cfg.Targets[0], testLogger())
	tg.collectOnce(context.Background())
	g := gather(t, tg)
	if v := g.find("vmware_up", nil); len(v) != 1 || value(v[0]) != 0 {
		t.Fatalf("vmware_up attendu à 0")
	}
	if v := g.find("vmware_collection_failures_total", nil); len(v) != 1 || value(v[0]) != 1 {
		t.Fatalf("compteur d'échec attendu à 1")
	}
}

// TestAppliance simule l'API REST /api d'une appliance vCenter.
func TestAppliance(t *testing.T) {
	_, s := newModel(t, "9.0.0", "9.0.0.0")
	logins := 0
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session" && r.Method == http.MethodPost {
			if u, p, ok := r.BasicAuth(); !ok || u == "" || p == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			logins++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode("tok123")
			return
		}
		if r.Header.Get("vmware-api-session-id") != "tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/appliance/health/system":
			_, _ = io.WriteString(w, `"orange"`)
		case r.URL.Path == "/api/appliance/health/database-storage":
			w.WriteHeader(http.StatusNotFound) // absent sur cette version
		case strings.HasPrefix(r.URL.Path, "/api/appliance/health/"):
			_, _ = io.WriteString(w, `"green"`)
		case r.URL.Path == "/api/appliance/system/version":
			_, _ = io.WriteString(w, `{"version":"9.0.0.0","build":"24755230","product":"VMware vCenter Server","type":"vCenter Server with an embedded Platform Services Controller"}`)
		case r.URL.Path == "/api/appliance/system/uptime":
			_, _ = io.WriteString(w, `123456.7`)
		case r.URL.Path == "/api/vcenter/services":
			_, _ = io.WriteString(w, `{"vpxd":{"state":"STARTED","health":"HEALTHY","startup_type":"AUTOMATIC"},"vsphere-ui":{"state":"STOPPED","health":"DEGRADED","startup_type":"AUTOMATIC"}}`)
		case r.URL.Path == "/api/appliance/recovery/backup/job/details":
			_, _ = io.WriteString(w, `{"a":{"status":"SUCCEEDED","end_time":"2026-10-14T01:00:00Z"},"b":{"status":"FAILED","end_time":"2026-10-15T01:00:00Z"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	cfg := targetFor(t, s, nil)
	tg := NewTarget(cfg, testLogger())
	ctx := context.Background()
	vc, err := tg.client.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Le REST pointe vers le faux serveur /api.
	restCfg := *cfg
	restCfg.URL = api.URL + "/sdk"
	r := &Run{Cfg: cfg, Log: testLogger(), Vim: vc, Sink: NewSink("vc-test", nil)}
	r.Client = newTestClientWithREST(t, &restCfg)
	if err := collectAppliance(ctx, r); err != nil {
		t.Fatal(err)
	}
	mt := map[string]float64{}
	for _, m := range r.Sink.Metrics() {
		var d dto.Metric
		_ = m.Write(&d)
		name := descName(m.Desc().String())
		switch name {
		case "vmware_appliance_health":
			mt["health_"+label(&d, "component")] = value(&d)
		case "vmware_appliance_service_started":
			mt["started_"+label(&d, "service")] = value(&d)
		case "vmware_appliance_backup_last_succeeded", "vmware_appliance_uptime_seconds":
			mt[name] = value(&d)
		case "vmware_appliance_endpoint_up":
			mt["ep_"+label(&d, "endpoint")] = value(&d)
		}
	}
	checks := map[string]float64{
		"health_system": 2, "health_mem": 1, "started_vpxd": 1, "started_vsphere-ui": 0,
		"vmware_appliance_backup_last_succeeded": 0, "vmware_appliance_uptime_seconds": 123456.7,
		"ep_health/database-storage": 0,
	}
	for k, want := range checks {
		if got, ok := mt[k]; !ok || got != want {
			t.Errorf("%s = %v (présent=%v), attendu %v", k, got, ok, want)
		}
	}
	if logins != 1 {
		t.Errorf("sessions REST ouvertes: %d, attendu 1 (réutilisation)", logins)
	}
}

func descName(d string) string {
	// Desc{fqName: "x", ...}
	i := strings.Index(d, `fqName: "`)
	if i < 0 {
		return ""
	}
	d = d[i+9:]
	return d[:strings.Index(d, `"`)]
}

func boolp(v bool) *bool { return &v }

type sessionKiller struct{}

func (sessionKiller) kill(ctx context.Context, vc *vim25.Client) error {
	return session.NewManager(vc).Logout(ctx)
}

func newTestClientWithREST(t *testing.T, cfg *config.Target) *vsphere.Client {
	t.Helper()
	return vsphere.New(cfg, testLogger())
}

func TestPerfAggregate(t *testing.T) {
	vals := []int64{10, -1, 50, 20}
	cases := map[types.PerfSummaryType]float64{
		types.PerfSummaryTypeAverage: 80.0 / 3, types.PerfSummaryTypeSummation: 80.0 / 3,
		types.PerfSummaryTypeMaximum: 50, types.PerfSummaryTypeMinimum: 10, types.PerfSummaryTypeLatest: 20,
	}
	for rollup, want := range cases {
		got, ok := aggregate(vals, rollup)
		if !ok || got != want {
			t.Errorf("%s: %v, attendu %v", rollup, got, want)
		}
	}
	if _, ok := aggregate([]int64{-1, -1}, types.PerfSummaryTypeAverage); ok {
		t.Errorf("série vide doit être ignorée")
	}
}
