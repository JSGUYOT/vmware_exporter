// Package collector contient les collecteurs vSphere et le cache exposé à Prometheus.
package collector

import (
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/vim25/types"
)

const namespace = "vmware"

// Sink accumule les métriques d'une collecte (thread-safe).
type Sink struct {
	mu      sync.Mutex
	metrics []prometheus.Metric
	descs   map[string]*prometheus.Desc
	// labels communs (vcenter + extra_labels) préfixés à chaque série
	baseNames  []string
	baseValues []string
	seen       map[string]struct{}
	dups       int
	errs       int
	// Log optionnel pour signaler les séries invalides.
	Log *slog.Logger
}

// NewSink crée un sink avec les labels communs d'une cible.
func NewSink(vcenter string, extra map[string]string) *Sink {
	s := &Sink{descs: map[string]*prometheus.Desc{}, seen: map[string]struct{}{}}
	s.baseNames = []string{"vcenter"}
	s.baseValues = []string{vcenter}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.baseNames = append(s.baseNames, k)
		s.baseValues = append(s.baseValues, extra[k])
	}
	return s
}

func (s *Sink) desc(name, help string, labels []string) *prometheus.Desc {
	key := name + "|" + strings.Join(labels, ",")
	if d, ok := s.descs[key]; ok {
		return d
	}
	all := append(append([]string{}, s.baseNames...), labels...)
	d := prometheus.NewDesc(name, help, all, nil)
	s.descs[key] = d
	return d
}

// L est un raccourci pour une liste de paires label=valeur.
type L []string

func split(l L) (names, values []string) {
	for i := 0; i+1 < len(l); i += 2 {
		names = append(names, l[i])
		values = append(values, l[i+1])
	}
	return
}

func (s *Sink) add(t prometheus.ValueType, name, help string, v float64, l L) {
	names, values := split(l)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Protection contre les doublons (deux objets de même nom au même endroit) :
	// un doublon ferait échouer le scrape entier côté client_golang.
	key := name + "\xff" + strings.Join(values, "\xff")
	if _, dup := s.seen[key]; dup {
		s.dups++
		return
	}
	s.seen[key] = struct{}{}
	d := s.desc(name, help, names)
	m, err := prometheus.NewConstMetric(d, t, v, append(append([]string{}, s.baseValues...), values...)...)
	if err != nil {
		s.errs++
		if s.errs <= 5 && s.Log != nil {
			s.Log.Error("série invalide ignorée", "metric", name, "err", err)
		}
		return
	}
	s.metrics = append(s.metrics, m)
}

// Gauge ajoute une jauge.
func (s *Sink) Gauge(name, help string, v float64, l L) {
	s.add(prometheus.GaugeValue, name, help, v, l)
}

// Counter ajoute un compteur.
func (s *Sink) Counter(name, help string, v float64, l L) {
	s.add(prometheus.CounterValue, name, help, v, l)
}

// Metrics renvoie les métriques collectées.
func (s *Sink) Metrics() []prometheus.Metric {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

// Errors renvoie le nombre de séries invalides ignorées.
func (s *Sink) Errors() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errs
}

// Duplicates renvoie le nombre de séries ignorées car dupliquées.
func (s *Sink) Duplicates() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dups
}

func n(name string) string { return namespace + "_" + name }

func b2f(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func bp2f(v *bool) float64 {
	if v == nil {
		return 0
	}
	return b2f(*v)
}

// Statut global vSphere : 0=gray(inconnu) 1=green 2=yellow 3=red.
const statusHelp = " (0=gray/inconnu, 1=green, 2=yellow, 3=red)"

func statusValue(s types.ManagedEntityStatus) float64 {
	switch s {
	case types.ManagedEntityStatusGreen:
		return 1
	case types.ManagedEntityStatusYellow:
		return 2
	case types.ManagedEntityStatusRed:
		return 3
	}
	return 0
}

// healthString pour les statuts texte REST/vSAN (green/yellow/orange/red/gray/unknown/info/skipped).
func healthString(s string) float64 {
	switch strings.ToLower(s) {
	case "green", "ok", "healthy", "info", "skipped", "passed":
		return 1
	case "yellow", "warning", "orange":
		return 2
	case "red", "error", "critical", "failed":
		return 3
	}
	return 0
}

const powerHelp = " (0=poweredOff, 1=poweredOn, 2=suspended/standBy, 3=unknown)"

func vmPower(s types.VirtualMachinePowerState) float64 {
	switch s {
	case types.VirtualMachinePowerStatePoweredOff:
		return 0
	case types.VirtualMachinePowerStatePoweredOn:
		return 1
	case types.VirtualMachinePowerStateSuspended:
		return 2
	}
	return 3
}

func hostPower(s types.HostSystemPowerState) float64 {
	switch s {
	case types.HostSystemPowerStatePoweredOff:
		return 0
	case types.HostSystemPowerStatePoweredOn:
		return 1
	case types.HostSystemPowerStateStandBy:
		return 2
	}
	return 3
}

const hostConnHelp = " (0=disconnected, 1=connected, 2=notResponding)"

func hostConn(s types.HostSystemConnectionState) float64 {
	switch s {
	case types.HostSystemConnectionStateConnected:
		return 1
	case types.HostSystemConnectionStateNotResponding:
		return 2
	}
	return 0
}

const vmConnHelp = " (0=connected, 1=disconnected, 2=orphaned, 3=inaccessible, 4=invalid)"

func vmConn(s types.VirtualMachineConnectionState) float64 {
	switch s {
	case types.VirtualMachineConnectionStateConnected:
		return 0
	case types.VirtualMachineConnectionStateDisconnected:
		return 1
	case types.VirtualMachineConnectionStateOrphaned:
		return 2
	case types.VirtualMachineConnectionStateInaccessible:
		return 3
	}
	return 4
}
