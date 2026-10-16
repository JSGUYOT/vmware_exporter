// Command vmware-exporter exposes, in Prometheus format, the full state of one
// or more vCenter 8.x / 9.x (VCF 9) servers: resources, health and alarms.
package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vmware/govmomi/vim25"

	"github.com/JSGUYOT/vmware_exporter/internal/collector"
	"github.com/JSGUYOT/vmware_exporter/internal/config"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfgPath := flag.String("config.file", "/etc/vmware-exporter/config.yml", "YAML configuration file.")
	check := flag.Bool("config.check", false, "Validate the configuration and exit.")
	showVersion := flag.Bool("version", false, "Print the version.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vmware-exporter %s (commit %s, govmomi vim25 %s)\n", version, commit, vim25.Version)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid configuration:", err)
		os.Exit(1)
	}
	if *check {
		fmt.Printf("configuration OK: %d vCenter(s)\n", len(cfg.Targets))
		return
	}
	log := newLogger(cfg.LogLevel, cfg.LogFormat)
	log.Info("starting vmware-exporter", "version", version, "targets", len(cfg.Targets), "listen", cfg.ListenAddress)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	exp := &collector.Exporter{}
	for i := range cfg.Targets {
		t := collector.NewTarget(&cfg.Targets[i], log)
		exp.Targets = append(exp.Targets, t)
		t.Start(ctx)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(exp,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "vmware_exporter_build_info", Help: "Exporter version (value is always 1).",
			ConstLabels: prometheus.Labels{"version": version, "commit": commit, "govmomi_api": vim25.Version},
		}, func() float64 { return 1 }),
	)

	opts := promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError), ErrorHandling: promhttp.ContinueOnError}
	all := promhttp.HandlerFor(reg, opts)
	// serve answers with the given vCenters only (all of them when names is empty).
	serve := func(w http.ResponseWriter, r *http.Request, names []string) {
		if len(names) == 0 {
			all.ServeHTTP(w, r)
			return
		}
		sub := &collector.Exporter{}
		for _, s := range names {
			t := exp.Lookup(s)
			if t == nil {
				http.Error(w, fmt.Sprintf("unknown target %q: not in the configuration", s), http.StatusNotFound)
				return
			}
			sub.Targets = append(sub.Targets, t)
		}
		sreg := prometheus.NewRegistry()
		sreg.MustRegister(sub)
		promhttp.HandlerFor(sreg, opts).ServeHTTP(w, r)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(cfg.TelemetryPath, func(w http.ResponseWriter, r *http.Request) {
		// ?target=<name|host> (repeatable, comma-separated) or ?vsphere_host=<host>
		// (pryorda/vmware_exporter compatible) restricts the response to some vCenters.
		q := r.URL.Query()
		var names []string
		for _, v := range append(q["target"], q["vsphere_host"]...) {
			for _, s := range strings.Split(v, ",") {
				if s = strings.TrimSpace(s); s != "" {
					names = append(names, s)
				}
			}
		}
		serve(w, r, names)
	})
	// One endpoint per vCenter: <telemetry_path>/<name|host>, e.g. /metrics/vc8-site-a.
	perTarget := strings.TrimSuffix(cfg.TelemetryPath, "/")
	mux.HandleFunc(perTarget+"/{target}", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, []string{r.PathValue("target")})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		for _, t := range cfg.Targets {
			fmt.Fprintf(&b, `<li><a href="%s/%s">%s</a> (interval %s)</li>`,
				perTarget, url.PathEscape(t.Name), html.EscapeString(t.Name), t.Interval)
		}
		fmt.Fprintf(w, `<html><head><title>VMware Exporter</title></head><body><h1>VMware Exporter %s</h1>
<p><a href="%s">Metrics (all vCenters)</a></p><ul>%s</ul></body></html>`, version, cfg.TelemetryPath, b.String())
	})

	srv := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if cfg.TLSServer != nil && cfg.TLSServer.CertFile != "" {
		err = srv.ListenAndServeTLS(cfg.TLSServer.CertFile, cfg.TLSServer.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		log.Error("HTTP server stopped", "err", err)
		os.Exit(1)
	}
	time.Sleep(500 * time.Millisecond) // let logouts complete
	log.Info("stopped")
}

func newLogger(level, format string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lv}
	if strings.EqualFold(format, "json") {
		return slog.New(slog.NewJSONHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, opts))
}
