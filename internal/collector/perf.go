package collector

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/types"
)

type perfEntity struct {
	ref    types.ManagedObjectReference
	labels L
}

type unitConv struct {
	suffix string
	factor float64
}

// Conversion to readable units close to Prometheus conventions.
var perfUnits = map[string]unitConv{
	"percent":            {"percent", 0.01}, // vSphere returns hundredths of a percent
	"kiloBytes":          {"bytes", 1024},
	"megaBytes":          {"bytes", 1024 * 1024},
	"teraBytes":          {"bytes", 1 << 40},
	"kiloBytesPerSecond": {"bytes_per_second", 1024},
	"megaBytesPerSecond": {"bytes_per_second", 1024 * 1024},
	"megaHertz":          {"mhz", 1},
	"millisecond":        {"seconds", 0.001},
	"microsecond":        {"seconds", 0.000001},
	"second":             {"seconds", 1},
	"watt":               {"watts", 1},
	"joule":              {"joules", 1},
	"celsius":            {"celsius", 1},
	"number":             {"", 1},
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9_]+`)

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return nonAlnum.ReplaceAllString(b.String(), "_")
}

type wantedCounter struct {
	info     *types.PerfCounterInfo
	allInst  bool
	metric   string
	factor   float64
	fullName string
}

// collectPerf queries real-time counters (20s interval) of powered-on hosts and VMs.
func collectPerf(ctx context.Context, r *Run) error {
	pm := performance.NewManager(r.Vim)
	byName, err := pm.CounterInfoByName(ctx)
	if err != nil {
		return err
	}
	maxSample := int32(r.Cfg.Interval.Seconds() / 20)
	if maxSample < 1 {
		maxSample = 1
	}
	if maxSample > 15 {
		maxSample = 15
	}

	resolve := func(kind string, names []string) []wantedCounter {
		var out []wantedCounter
		for _, raw := range names {
			name, all := strings.TrimSuffix(raw, ":*"), strings.HasSuffix(raw, ":*")
			info, ok := byName[name]
			if !ok {
				r.Log.Debug("performance counter unknown on this vCenter, skipped", "counter", name)
				continue
			}
			unit := info.UnitInfo.GetElementDescription().Key
			uc, ok := perfUnits[unit]
			if !ok {
				uc = unitConv{snake(unit), 1}
			}
			parts := strings.Split(name, ".")
			m := n(kind + "_perf_" + strings.Join(func() []string {
				o := make([]string, len(parts))
				for i, p := range parts {
					o[i] = snake(p)
				}
				return o
			}(), "_"))
			if uc.suffix != "" && !strings.HasSuffix(m, "_"+uc.suffix) {
				m += "_" + uc.suffix
			}
			out = append(out, wantedCounter{info: info, allInst: all, metric: m, factor: uc.factor, fullName: name})
		}
		return out
	}

	run := func(kind string, ents []perfEntity, wanted []wantedCounter) error {
		if len(ents) == 0 || len(wanted) == 0 {
			return nil
		}
		byID := map[int32]wantedCounter{}
		var ids []types.PerfMetricId
		for _, w := range wanted {
			inst := ""
			if w.allInst {
				inst = "*"
			}
			byID[w.info.Key] = w
			ids = append(ids, types.PerfMetricId{CounterId: w.info.Key, Instance: inst})
		}
		labelsOf := map[string]L{}
		for _, e := range ents {
			labelsOf[e.ref.Value] = e.labels
		}
		bs := r.Cfg.Perf.BatchSize
		sem := make(chan struct{}, r.Cfg.Perf.MaxConcurrency)
		var wg sync.WaitGroup
		var firstErr error
		var emu sync.Mutex
		for i := 0; i < len(ents); i += bs {
			end := i + bs
			if end > len(ents) {
				end = len(ents)
			}
			specs := make([]types.PerfQuerySpec, 0, end-i)
			for _, e := range ents[i:end] {
				specs = append(specs, types.PerfQuerySpec{
					Entity: e.ref, MetricId: ids, IntervalId: 20, MaxSample: maxSample, Format: string(types.PerfFormatNormal),
				})
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(specs []types.PerfQuerySpec) {
				defer wg.Done()
				defer func() { <-sem }()
				res, err := pm.Query(ctx, specs)
				if err != nil && len(specs) > 1 {
					// An entity that vanished (VM powered off/deleted, host disconnected
					// between inventory and query) fails the whole batch: retry one
					// entity at a time so only that one is lost.
					r.Log.Debug("QueryPerf batch failed, retrying one entity at a time", "kind", kind, "size", len(specs), "err", err)
					res, err = nil, nil
					failed := 0
					var lastErr error
					for _, sp := range specs {
						one, e := pm.Query(ctx, []types.PerfQuerySpec{sp})
						if e != nil {
							failed++
							lastErr = e
							continue
						}
						res = append(res, one...)
					}
					if failed == len(specs) {
						err = lastErr
					}
				}
				if err != nil {
					emu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					emu.Unlock()
					return
				}
				for _, base := range res {
					em, ok := base.(*types.PerfEntityMetric)
					if !ok {
						continue
					}
					l := labelsOf[em.Entity.Value]
					for _, sb := range em.Value {
						series, ok := sb.(*types.PerfMetricIntSeries)
						if !ok {
							continue
						}
						w, ok := byID[series.Id.CounterId]
						if !ok {
							continue
						}
						v, ok := aggregate(series.Value, w.info.RollupType)
						if !ok {
							continue
						}
						help := "Real-time vSphere counter " + w.fullName + " (" + aggHelp(w.info.RollupType) + " of 20s samples since the last collection)."
						r.Sink.Gauge(w.metric, help, v*w.factor, append(append(L{}, l...), "perf_instance", series.Id.Instance))
					}
				}
			}(specs)
		}
		wg.Wait()
		return firstErr
	}

	r.mu.Lock()
	hosts, vms := r.hostsPerf, r.vmsPerf
	r.mu.Unlock()
	errH := run("host", hosts, resolve("host", r.Cfg.Perf.HostCounters))
	errV := run("vm", vms, resolve("vm", r.Cfg.Perf.VMCounters))
	if errH != nil {
		return errH
	}
	return errV
}

// aggregate summarises the 20s samples according to the counter's rollup type:
// "latest" -> last sample, "maximum"/"minimum" -> extreme (peaks are not
// smoothed), otherwise average. -1 values (missing sample) are skipped.
func aggregate(vals []int64, rollup types.PerfSummaryType) (float64, bool) {
	var sum, out float64
	cnt := 0
	for _, v := range vals {
		if v < 0 {
			continue
		}
		f := float64(v)
		switch {
		case cnt == 0:
			out = f
		case rollup == types.PerfSummaryTypeMaximum && f > out:
			out = f
		case rollup == types.PerfSummaryTypeMinimum && f < out:
			out = f
		case rollup == types.PerfSummaryTypeLatest:
			out = f
		}
		sum += f
		cnt++
	}
	if cnt == 0 {
		return 0, false
	}
	switch rollup {
	case types.PerfSummaryTypeMaximum, types.PerfSummaryTypeMinimum, types.PerfSummaryTypeLatest:
		return out, true
	}
	return sum / float64(cnt), true
}

func aggHelp(rollup types.PerfSummaryType) string {
	switch rollup {
	case types.PerfSummaryTypeMaximum:
		return "maximum"
	case types.PerfSummaryTypeMinimum:
		return "minimum"
	case types.PerfSummaryTypeLatest:
		return "latest"
	}
	return "average"
}
