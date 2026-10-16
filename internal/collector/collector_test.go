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

// newModel creates a simulated vCenter advertising the given version (8.0.3 or 9.0.0).
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

// TestCompatibilityMatrix runs a full collection against simulated vCenter 8 and vCenter 9.
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
				t.Fatalf("expected vmware_up to be 1")
			}
			info := g.find("vmware_vcenter_info", nil)
			if len(info) != 1 {
				t.Fatalf("vmware_vcenter_info missing")
			}
			if got := label(info[0], "api_version_used"); got != tc.api {
				t.Errorf("negotiation: api_version_used=%q, want %q", got, tc.api)
			}
			if got := label(info[0], "site"); got != "lab" {
				t.Errorf("extra_labels not applied: %q", got)
			}
			for _, m := range g.find("vmware_collector_success", nil) {
				if value(m) != 1 {
					t.Errorf("collector %s failed", label(m, "collector"))
				}
			}
			if u := g.find("vmware_exporter_unsupported_properties", nil); len(u) == 1 && value(u[0]) != 0 {
				t.Errorf("%v properties rejected by the simulator", value(u[0]))
			}
			al := g.find("vmware_alarm_triggered", map[string]string{"alarm": "vcsim VM Alarm", "entity": "DC0_C0_RP0_VM0", "status": "red"})
			if len(al) != 1 || value(al[0]) != 3 {
				t.Errorf("red vCenter alarm not reported (%d series)", len(al))
			} else if label(al[0], "datacenter") != "DC0" {
				t.Errorf("alarm datacenter = %q", label(al[0], "datacenter"))
			}
			mustHave := []string{
				"vmware_cluster_overall_status", "vmware_cluster_cpu_total_mhz", "vmware_cluster_ha_enabled",
				"vmware_host_connection_status", "vmware_host_info", "vmware_host_memory_usage_bytes",
				"vmware_vm_power_status", "vmware_vm_info", "vmware_vm_snapshots_existing", "vmware_vm_storage_committed_bytes",
				"vmware_datastore_capacity_bytes", "vmware_datastore_is_accessible", "vmware_datastore_cluster_overall_status",
				"vmware_dvs_overall_status", "vmware_network_overall_status", "vmware_resourcepool_overall_status",
				"vmware_datacenter_overall_status", "vmware_inventory_objects",
				"vmware_host_perf_cpu_usage_average_percent", "vmware_vm_perf_cpu_ready_summation_seconds",
			}
			for _, name := range mustHave {
				if _, ok := g.fams[name]; !ok {
					t.Errorf("metric %s missing", name)
				}
			}
			vm := g.find("vmware_vm_power_status", map[string]string{"vm": "DC0_C0_RP0_VM0"})
			if len(vm) != 1 || label(vm[0], "datacenter") != "DC0" || label(vm[0], "host") != "" || label(vm[0], "cluster") != "" {
				t.Errorf("expected stable VM labels (no host/cluster): %v", vm)
			}
			vi := g.find("vmware_vm_info", map[string]string{"vm": "DC0_C0_RP0_VM0"})
			if len(vi) != 1 || label(vi[0], "cluster") != "DC0_C0" || label(vi[0], "host") == "" {
				t.Errorf("vmware_vm_info must carry cluster/host: %v", vi)
			}
			h := g.find("vmware_host_connection_status", map[string]string{"cluster": ""})
			if len(h) == 0 {
				t.Errorf("standalone host (no cluster) not reported")
			}
		})
	}
}

// TestLint checks Prometheus naming conventions.
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

// TestInvalidPropertyFallback checks parsing of an InvalidProperty fault
// (returned by a real vCenter when a property does not exist in its API version).
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
		t.Fatalf("remove sub-paths=%v", got)
	}
}

// TestSessionRecovery checks automatic reconnection after session expiry.
func TestSessionRecovery(t *testing.T) {
	_, s := newModel(t, "9.0.0", "9.0.0.0")
	cfg := targetFor(t, s, func(c *config.Target) { c.Collectors.Appliance = boolp(false) })
	tg := NewTarget(cfg, testLogger())
	ctx := context.Background()
	tg.collectOnce(ctx)
	vc, _ := tg.client.Get(ctx)
	// Simulate a vpxd restart: the current session is destroyed server-side.
	if err := (&sessionKiller{}).kill(ctx, vc); err != nil {
		t.Fatal(err)
	}
	tg.collectOnce(ctx)
	g := gather(t, tg)
	if v := g.find("vmware_up", nil); len(v) != 1 || value(v[0]) != 1 {
		t.Fatalf("no reconnection after session expiry")
	}
}

// TestUnreachable checks that an unreachable vCenter yields vmware_up=0 without crashing.
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
		t.Fatalf("expected vmware_up to be 0")
	}
	if v := g.find("vmware_collection_failures_total", nil); len(v) != 1 || value(v[0]) != 1 {
		t.Fatalf("expected failure counter to be 1")
	}
}

// TestAppliance simulates the /api REST API of a vCenter appliance.
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
			w.WriteHeader(http.StatusNotFound) // not available on this version
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
	// Point REST at the fake /api server.
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
			t.Errorf("%s = %v (present=%v), want %v", k, got, ok, want)
		}
	}
	if logins != 1 {
		t.Errorf("REST sessions opened: %d, want 1 (reuse)", logins)
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
			t.Errorf("%s: %v, want %v", rollup, got, want)
		}
	}
	if _, ok := aggregate([]int64{-1, -1}, types.PerfSummaryTypeAverage); ok {
		t.Errorf("empty series must be skipped")
	}
}

func TestLookup(t *testing.T) {
	cfgs := []config.Target{
		{Name: "vc8-site-a", URL: "https://vcenter-a.example.local/sdk"},
		{Name: "vcf9-mgmt", URL: "https://vcf9-mgmt-vc.example.local/sdk"},
	}
	e := &Exporter{}
	for i := range cfgs {
		e.Targets = append(e.Targets, NewTarget(&cfgs[i], testLogger()))
	}
	for s, want := range map[string]string{
		"vc8-site-a": "vc8-site-a", "VCF9-MGMT": "vcf9-mgmt",
		"vcenter-a.example.local": "vc8-site-a", "vcf9-mgmt-vc.example.local": "vcf9-mgmt",
		"unknown": "", "https://vcenter-a.example.local/sdk": "",
	} {
		got := ""
		if tg := e.Lookup(s); tg != nil {
			got = tg.Name()
		}
		if got != want {
			t.Errorf("Lookup(%q) = %q, want %q", s, got, want)
		}
	}
}

// TestPryordaCompat checks the pryorda/vmware_exporter compatible series:
// names, exact label sets, value semantics, and coexistence with the native
// families (the pedantic registry rejects inconsistent help texts).
func TestPryordaCompat(t *testing.T) {
	for _, v := range []struct{ version, api string }{{"8.0.3", "8.0.3.0"}, {"9.0.0", "9.0.0.0"}} {
		t.Run(v.version, func(t *testing.T) {
			_, s := newModel(t, v.version, v.api)
			cfg := targetFor(t, s, func(c *config.Target) { on := true; c.Collectors.Pryorda = &on })
			tg := NewTarget(cfg, testLogger())
			tg.collectOnce(context.Background())
			g := gather(t, tg)

			base := []string{"vcenter", "site"}
			vmL := append(append([]string{}, base...), "vm_name", "ds_name", "host_name", "dc_name", "cluster_name")
			hostL := append(append([]string{}, base...), "host_name", "dc_name", "cluster_name")
			dsL := append(append([]string{}, base...), "ds_name", "dc_name", "ds_cluster")
			want := map[string][]string{
				"vmware_vm_power_state":                vmL,
				"vmware_vm_num_cpu":                    vmL,
				"vmware_vm_memory_max":                 vmL,
				"vmware_vm_max_cpu_usage":              vmL,
				"vmware_vm_template":                   vmL,
				"vmware_vm_guest_tools_running_status": append(append([]string{}, vmL...), "tools_status"),
				"vmware_vm_cpu_usage_average":          vmL,
				"vmware_host_power_state":              hostL,
				"vmware_host_maintenance_mode":         hostL,
				"vmware_host_connection_state":         append(append([]string{}, hostL...), "state"),
				"vmware_host_standby_mode":             append(append([]string{}, hostL...), "standby_mode_state"),
				"vmware_host_cpu_usage":                hostL,
				"vmware_host_cpu_max":                  hostL,
				"vmware_host_num_cpu":                  hostL,
				"vmware_host_memory_usage":             hostL,
				"vmware_host_memory_max":               hostL,
				"vmware_host_product_info":             append(append([]string{}, hostL...), "version", "build"),
				"vmware_host_hardware_info":            append(append([]string{}, hostL...), "hardware_model", "hardware_cpu_model"),
				"vmware_host_cpu_usage_average":        hostL,
				"vmware_datastore_capacity_size":       dsL,
				"vmware_datastore_freespace_size":      dsL,
				"vmware_datastore_uncommited_size":     dsL,
				"vmware_datastore_provisoned_size":     dsL,
				"vmware_datastore_hosts":               dsL,
				"vmware_datastore_vms":                 dsL,
				"vmware_datastore_accessible":          dsL,
				"vmware_datastore_maintenance_mode":    append(append([]string{}, dsL...), "mode"),
				"vmware_datastore_type":                append(append([]string{}, dsL...), "ds_type"),
			}
			for name, labels := range want {
				f, ok := g.fams[name]
				if !ok || len(f.Metric) == 0 {
					t.Errorf("%s: missing", name)
					continue
				}
				got := map[string]bool{}
				for _, lp := range f.Metric[0].Label {
					got[lp.GetName()] = true
				}
				for _, l := range labels {
					if !got[l] {
						t.Errorf("%s: label %q missing", name, l)
					}
				}
				if len(got) != len(labels) {
					t.Errorf("%s: got %d labels, want %d (%v)", name, len(got), len(labels), labels)
				}
			}

			vm := g.find("vmware_vm_power_state", map[string]string{"vm_name": "DC0_C0_RP0_VM0", "dc_name": "DC0", "cluster_name": "DC0_C0"})
			if len(vm) != 1 || value(vm[0]) != 1 || label(vm[0], "ds_name") == "" || label(vm[0], "host_name") == "" {
				t.Errorf("vmware_vm_power_state for DC0_C0_RP0_VM0: %v", vm)
			}
			if m := g.find("vmware_vm_memory_max", map[string]string{"vm_name": "DC0_C0_RP0_VM0"}); len(m) != 1 || value(m[0]) < 32 {
				t.Errorf("vmware_vm_memory_max should be in MB: %v", m)
			}
			if _, ok := g.fams["vmware_vm_snapshots"]; ok {
				t.Error("vmware_vm_snapshots must be absent for VMs without snapshots")
			}
			for _, m := range g.fams["vmware_datastore_provisoned_size"].Metric {
				ds := label(m, "ds_name")
				c := g.find("vmware_datastore_capacity_size", map[string]string{"ds_name": ds})
				f := g.find("vmware_datastore_freespace_size", map[string]string{"ds_name": ds})
				u := g.find("vmware_datastore_uncommited_size", map[string]string{"ds_name": ds})
				if value(m) != value(c[0])-value(f[0])+value(u[0]) {
					t.Errorf("provisoned_size of %s is not capacity - free + uncommitted", ds)
				}
			}
			// Native families keep their own names next to the pryorda ones.
			for _, name := range []string{"vmware_vm_power_status", "vmware_host_connection_status", "vmware_datastore_is_accessible"} {
				if _, ok := g.fams[name]; !ok {
					t.Errorf("native %s missing in pryorda mode", name)
				}
			}
		})
	}
}
