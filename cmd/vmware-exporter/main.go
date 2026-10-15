// Command vmware-exporter expose au format Prometheus l'état complet d'un ou
// plusieurs vCenter 8.x / 9.x (VCF 9) : ressources, santé et alarmes.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vmware/govmomi/vim25"

	"github.com/example/vmware-exporter/internal/collector"
	"github.com/example/vmware-exporter/internal/config"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	cfgPath := flag.String("config.file", "/etc/vmware-exporter/config.yml", "Fichier de configuration YAML.")
	check := flag.Bool("config.check", false, "Valide la configuration puis quitte.")
	showVersion := flag.Bool("version", false, "Affiche la version.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vmware-exporter %s (commit %s, govmomi vim25 %s)\n", version, commit, vim25.Version)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration invalide:", err)
		os.Exit(1)
	}
	if *check {
		fmt.Printf("configuration OK : %d vCenter(s)\n", len(cfg.Targets))
		return
	}
	log := newLogger(cfg.LogLevel, cfg.LogFormat)
	log.Info("démarrage de vmware-exporter", "version", version, "targets", len(cfg.Targets), "listen", cfg.ListenAddress)

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
			Name: "vmware_exporter_build_info", Help: "Version de l'exporter (valeur toujours 1).",
			ConstLabels: prometheus.Labels{"version": version, "commit": commit, "govmomi_api": vim25.Version},
		}, func() float64 { return 1 }),
	)

	mux := http.NewServeMux()
	mux.Handle(cfg.TelemetryPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError), ErrorHandling: promhttp.ContinueOnError}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		for _, t := range cfg.Targets {
			fmt.Fprintf(&b, "<li>%s (intervalle %s)</li>", t.Name, t.Interval)
		}
		fmt.Fprintf(w, `<html><head><title>VMware Exporter</title></head><body><h1>VMware Exporter %s</h1>
<p><a href="%s">Métriques</a></p><ul>%s</ul></body></html>`, version, cfg.TelemetryPath, b.String())
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
		log.Error("serveur HTTP arrêté", "err", err)
		os.Exit(1)
	}
	time.Sleep(500 * time.Millisecond) // laisse les logout se terminer
	log.Info("arrêt")
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
