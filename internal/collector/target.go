package collector

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"

	"github.com/JSGUYOT/vmware_exporter/internal/config"
	"github.com/JSGUYOT/vmware_exporter/internal/vsphere"
)

// Run holds the context of one collection.
type Run struct {
	Cfg    *config.Target
	Log    *slog.Logger
	Client *vsphere.Client
	Vim    *vim25.Client
	View   *view.ContainerView
	Topo   *Topology
	Sink   *Sink

	// Results shared between collectors (filled by host/vm, read by perf).
	mu        sync.Mutex
	hostsPerf []perfEntity
	vmsPerf   []perfEntity
	// pryorda compatibility: host labels by moid and VMs to emit, filled by the
	// host and vm collectors (phase 1), read by pryorda and perf (phase 2).
	pryHosts map[string]L
	pryVMs   []*pryordaVM

	vmExclude, vmInclude, hostExclude, dsExclude *regexp.Regexp
	unsupported, missing                         atomic.Int64
}

func (r *Run) addPerfHost(e perfEntity) {
	r.mu.Lock()
	r.hostsPerf = append(r.hostsPerf, e)
	r.mu.Unlock()
}
func (r *Run) addPerfVM(e perfEntity) { r.mu.Lock(); r.vmsPerf = append(r.vmsPerf, e); r.mu.Unlock() }

type collectorFunc struct {
	name    string
	enabled func(*config.Collectors) bool
	fn      func(context.Context, *Run) error
	// phase 1 = runs in parallel; phase 2 = after phase 1 (depends on its results)
	phase int
}

var collectors = []collectorFunc{
	{"vcenter", func(c *config.Collectors) bool { return *c.VCenter }, collectVCenter, 1},
	{"alarms", func(c *config.Collectors) bool { return *c.Alarms }, collectAlarms, 1},
	{"datacenter", func(c *config.Collectors) bool { return *c.Datacenter }, collectDatacenters, 1},
	{"cluster", func(c *config.Collectors) bool { return *c.Cluster }, collectClusters, 1},
	{"host", func(c *config.Collectors) bool { return *c.Host || *c.Perf || *c.Pryorda }, collectHosts, 1},
	{"vm", func(c *config.Collectors) bool { return *c.VM || *c.Perf || *c.Pryorda }, collectVMs, 1},
	{"datastore", func(c *config.Collectors) bool { return *c.Datastore || *c.Pryorda }, collectDatastores, 1},
	{"network", func(c *config.Collectors) bool { return *c.Network }, collectNetwork, 1},
	{"resourcepool", func(c *config.Collectors) bool { return *c.ResourcePool }, collectResourcePools, 1},
	{"vsan", func(c *config.Collectors) bool { return *c.VSAN }, collectVSAN, 1},
	{"appliance", func(c *config.Collectors) bool { return *c.Appliance }, collectAppliance, 1},
	{"perf", func(c *config.Collectors) bool { return *c.Perf }, collectPerf, 2},
	{"pryorda", func(c *config.Collectors) bool { return *c.Pryorda }, collectPryorda, 2},
}

// Target periodically collects one vCenter and caches the latest result.
type Target struct {
	cfg    *config.Target
	log    *slog.Logger
	client *vsphere.Client

	mu       sync.RWMutex
	snapshot []prometheus.Metric

	scrapes  atomic.Int64
	failures atomic.Int64
}

// NewTarget prepares a target.
func NewTarget(cfg *config.Target, log *slog.Logger) *Target {
	l := log.With("vcenter", cfg.Name)
	return &Target{cfg: cfg, log: l, client: vsphere.New(cfg, l)}
}

// Start runs the collection loop until the context is cancelled.
func (t *Target) Start(ctx context.Context) {
	go func() {
		t.collectOnce(ctx)
		tk := time.NewTicker(t.cfg.Interval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				lctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.client.Logout(lctx)
				cancel()
				return
			case <-tk.C:
				t.collectOnce(ctx)
			}
		}
	}()
}

func (t *Target) compile(re string) *regexp.Regexp {
	if re == "" {
		return nil
	}
	return regexp.MustCompile(re)
}

func (t *Target) collectOnce(parent context.Context) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, t.cfg.Timeout)
	defer cancel()
	t.scrapes.Add(1)

	sink := NewSink(t.cfg.Name, t.cfg.ExtraLabels)
	sink.Log = t.log
	self := *t.cfg.Collectors.Exporter
	up := 0.0
	defer func() {
		sink.Gauge(n("up"), "1 if the last collection of the vCenter succeeded (connection + inventory).", up, nil)
		if self {
			sink.Gauge(n("collection_duration_seconds"), "Total duration of the last collection.", time.Since(start).Seconds(), nil)
			sink.Gauge(n("collection_timestamp_seconds"), "Timestamp of the last collection.", float64(time.Now().Unix()), nil)
			sink.Gauge(n("collection_interval_seconds"), "Configured collection interval.", t.cfg.Interval.Seconds(), nil)
			sink.Gauge(n("collection_timeout_seconds"), "Configured collection timeout.", t.cfg.Timeout.Seconds(), nil)
			sink.Counter(n("collections_total"), "Number of collections performed.", float64(t.scrapes.Load()), nil)
			sink.Counter(n("collection_failures_total"), "Number of failed collections (vCenter unreachable / authentication).", float64(t.failures.Load()), nil)
			sink.Gauge(n("exporter_invalid_series"), "Invalid series dropped (e.g. label name conflict).", float64(sink.Errors()), nil)
			sink.Gauge(n("exporter_duplicate_series"), "Series dropped as duplicates (objects with the same name).", float64(sink.Duplicates()), nil)
		}
		t.mu.Lock()
		t.snapshot = sink.Metrics()
		t.mu.Unlock()
	}()

	vc, err := t.client.Get(ctx)
	if err != nil {
		t.failures.Add(1)
		t.log.Error("cannot connect to vCenter", "err", err)
		return
	}
	v, err := view.NewManager(vc).CreateContainerView(ctx, vc.ServiceContent.RootFolder, nil, true)
	if err != nil {
		if vsphere.IsAuthError(err) {
			t.client.Invalidate()
		}
		t.failures.Add(1)
		t.log.Error("cannot create ContainerView", "err", err)
		return
	}
	defer func() {
		dctx, c2 := context.WithTimeout(context.Background(), 10*time.Second)
		_ = v.Destroy(dctx)
		c2()
	}()

	r := &Run{
		Cfg: t.cfg, Log: t.log, Client: t.client, Vim: vc, View: v, Sink: sink,
		vmExclude: t.compile(t.cfg.Filters.VMExclude), vmInclude: t.compile(t.cfg.Filters.VMInclude),
		hostExclude: t.compile(t.cfg.Filters.HostExclude), dsExclude: t.compile(t.cfg.Filters.DatastoreExclude),
	}
	if err := r.loadTopology(ctx); err != nil {
		t.failures.Add(1)
		t.log.Error("cannot read inventory", "err", err)
		if vsphere.IsAuthError(err) {
			t.client.Invalidate()
		}
		return
	}
	up = 1

	runPhase := func(phase int) {
		var wg sync.WaitGroup
		for _, c := range collectors {
			if c.phase != phase || !c.enabled(&t.cfg.Collectors) {
				continue
			}
			wg.Add(1)
			go func(c collectorFunc) {
				defer wg.Done()
				cs := time.Now()
				err := safeRun(ctx, r, c.fn)
				ok := 1.0
				if err != nil {
					ok = 0
					t.log.Warn("collector failed", "collector", c.name, "err", err)
				}
				if self {
					sink.Gauge(n("collector_success"), "1 if the collector succeeded during the last collection.", ok, L{"collector", c.name})
					sink.Gauge(n("collector_duration_seconds"), "Duration of the collector during the last collection.", time.Since(cs).Seconds(), L{"collector", c.name})
				}
			}(c)
		}
		wg.Wait()
	}
	runPhase(1)
	runPhase(2)
	if self {
		sink.Gauge(n("exporter_unsupported_properties"), "Properties skipped because the vCenter API version does not provide them.", float64(r.unsupported.Load()), nil)
		sink.Gauge(n("exporter_missing_properties"), "Properties unavailable on some objects during the last collection (permissions, unreachable ESX host...).", float64(r.missing.Load()), nil)
	}
	t.log.Debug("collection finished", "duration", time.Since(start), "series", len(sink.Metrics()))
}

func safeRun(ctx context.Context, r *Run, fn func(context.Context, *Run) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.Log.Error("panic in collector", "panic", p)
			err = errPanic
		}
	}()
	return fn(ctx, r)
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errPanic = constErr("internal collector panic (see logs)")

// Metrics returns the latest snapshot.
func (t *Target) Metrics() []prometheus.Metric {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshot
}

// Name returns the configured target name (value of the "vcenter" label).
func (t *Target) Name() string { return t.cfg.Name }

// Matches reports whether s designates this target: its configured name or
// the host name of its URL (case-insensitive).
func (t *Target) Matches(s string) bool {
	if strings.EqualFold(s, t.cfg.Name) {
		return true
	}
	u, err := url.Parse(t.cfg.URL)
	return err == nil && strings.EqualFold(s, u.Hostname())
}

// Exporter is the prometheus.Collector serving the snapshots of its targets.
type Exporter struct{ Targets []*Target }

// Lookup returns the target designated by s (see Target.Matches), or nil.
func (e *Exporter) Lookup(s string) *Target {
	for _, t := range e.Targets {
		if t.Matches(s) {
			return t
		}
	}
	return nil
}

// Describe declares nothing: "unchecked" collector (dynamic series).
func (e *Exporter) Describe(chan<- *prometheus.Desc) {}

// Collect returns the cache, without any vCenter call during the scrape.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	for _, t := range e.Targets {
		for _, m := range t.Metrics() {
			ch <- m
		}
	}
}
