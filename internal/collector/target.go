package collector

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"

	"github.com/example/vmware-exporter/internal/config"
	"github.com/example/vmware-exporter/internal/vsphere"
)

// Run regroupe le contexte d'une collecte.
type Run struct {
	Cfg    *config.Target
	Log    *slog.Logger
	Client *vsphere.Client
	Vim    *vim25.Client
	View   *view.ContainerView
	Topo   *Topology
	Sink   *Sink

	// Résultats partagés entre collecteurs (remplis par host/vm, lus par perf).
	mu        sync.Mutex
	hostsPerf []perfEntity
	vmsPerf   []perfEntity

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
	// phase 1 = s'exécute en parallèle; phase 2 = après la phase 1 (dépend de ses résultats)
	phase int
}

var collectors = []collectorFunc{
	{"vcenter", func(c *config.Collectors) bool { return *c.VCenter }, collectVCenter, 1},
	{"alarms", func(c *config.Collectors) bool { return *c.Alarms }, collectAlarms, 1},
	{"datacenter", func(c *config.Collectors) bool { return *c.Datacenter }, collectDatacenters, 1},
	{"cluster", func(c *config.Collectors) bool { return *c.Cluster }, collectClusters, 1},
	{"host", func(c *config.Collectors) bool { return *c.Host || *c.Perf }, collectHosts, 1},
	{"vm", func(c *config.Collectors) bool { return *c.VM || *c.Perf }, collectVMs, 1},
	{"datastore", func(c *config.Collectors) bool { return *c.Datastore }, collectDatastores, 1},
	{"network", func(c *config.Collectors) bool { return *c.Network }, collectNetwork, 1},
	{"resourcepool", func(c *config.Collectors) bool { return *c.ResourcePool }, collectResourcePools, 1},
	{"vsan", func(c *config.Collectors) bool { return *c.VSAN }, collectVSAN, 1},
	{"appliance", func(c *config.Collectors) bool { return *c.Appliance }, collectAppliance, 1},
	{"perf", func(c *config.Collectors) bool { return *c.Perf }, collectPerf, 2},
}

// Target collecte périodiquement un vCenter et garde le dernier résultat en cache.
type Target struct {
	cfg    *config.Target
	log    *slog.Logger
	client *vsphere.Client

	mu       sync.RWMutex
	snapshot []prometheus.Metric

	scrapes  atomic.Int64
	failures atomic.Int64
}

// NewTarget prépare une cible.
func NewTarget(cfg *config.Target, log *slog.Logger) *Target {
	l := log.With("vcenter", cfg.Name)
	return &Target{cfg: cfg, log: l, client: vsphere.New(cfg, l)}
}

// Start lance la boucle de collecte jusqu'à l'annulation du contexte.
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
	up := 0.0
	defer func() {
		sink.Gauge(n("up"), "1 si la dernière collecte du vCenter a réussi (connexion + inventaire).", up, nil)
		sink.Gauge(n("collection_duration_seconds"), "Durée totale de la dernière collecte.", time.Since(start).Seconds(), nil)
		sink.Gauge(n("collection_timestamp_seconds"), "Horodatage de la dernière collecte.", float64(time.Now().Unix()), nil)
		sink.Counter(n("collections_total"), "Nombre de collectes effectuées.", float64(t.scrapes.Load()), nil)
		sink.Counter(n("collection_failures_total"), "Nombre de collectes en échec (vCenter injoignable / auth).", float64(t.failures.Load()), nil)
		sink.Gauge(n("exporter_invalid_series"), "Séries invalides ignorées (ex. conflit de nom de label).", float64(sink.Errors()), nil)
		sink.Gauge(n("exporter_duplicate_series"), "Séries ignorées car dupliquées (objets homonymes).", float64(sink.Duplicates()), nil)
		t.mu.Lock()
		t.snapshot = sink.Metrics()
		t.mu.Unlock()
	}()

	vc, err := t.client.Get(ctx)
	if err != nil {
		t.failures.Add(1)
		t.log.Error("connexion au vCenter impossible", "err", err)
		return
	}
	v, err := view.NewManager(vc).CreateContainerView(ctx, vc.ServiceContent.RootFolder, nil, true)
	if err != nil {
		if vsphere.IsAuthError(err) {
			t.client.Invalidate()
		}
		t.failures.Add(1)
		t.log.Error("création du ContainerView impossible", "err", err)
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
		t.log.Error("lecture de l'inventaire impossible", "err", err)
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
					t.log.Warn("collecteur en échec", "collector", c.name, "err", err)
				}
				sink.Gauge(n("collector_success"), "1 si le collecteur a réussi lors de la dernière collecte.", ok, L{"collector", c.name})
				sink.Gauge(n("collector_duration_seconds"), "Durée du collecteur lors de la dernière collecte.", time.Since(cs).Seconds(), L{"collector", c.name})
			}(c)
		}
		wg.Wait()
	}
	runPhase(1)
	runPhase(2)
	sink.Gauge(n("exporter_unsupported_properties"), "Propriétés ignorées car absentes de la version d'API du vCenter.", float64(r.unsupported.Load()), nil)
	sink.Gauge(n("exporter_missing_properties"), "Propriétés indisponibles sur certains objets lors de la dernière collecte (droits, ESX injoignable...).", float64(r.missing.Load()), nil)
	t.log.Debug("collecte terminée", "duration", time.Since(start), "series", len(sink.Metrics()))
}

func safeRun(ctx context.Context, r *Run, fn func(context.Context, *Run) error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			r.Log.Error("panic dans un collecteur", "panic", p)
			err = errPanic
		}
	}()
	return fn(ctx, r)
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errPanic = constErr("panic interne du collecteur (voir logs)")

// Metrics renvoie le dernier instantané.
func (t *Target) Metrics() []prometheus.Metric {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.snapshot
}

// Exporter est le prometheus.Collector qui sert les instantanés de toutes les cibles.
type Exporter struct{ Targets []*Target }

// Describe ne déclare rien : collecteur "unchecked" (séries dynamiques).
func (e *Exporter) Describe(chan<- *prometheus.Desc) {}

// Collect renvoie le cache, sans aucun appel vCenter pendant le scrape.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	for _, t := range e.Targets {
		for _, m := range t.Metrics() {
			ch <- m
		}
	}
}
