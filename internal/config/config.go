// Package config charge et valide la configuration YAML de l'exporter.
//
// Aucun secret ni URL n'est codé en dur : tout vient du fichier de
// configuration, des variables d'environnement (${VAR} interpolées) ou de
// fichiers de mots de passe dédiés.
package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config est la racine du fichier de configuration.
type Config struct {
	ListenAddress string   `yaml:"listen_address"`
	TelemetryPath string   `yaml:"telemetry_path"`
	LogLevel      string   `yaml:"log_level"`
	LogFormat     string   `yaml:"log_format"`
	TLSServer     *TLSSrv  `yaml:"web_tls"`
	Defaults      Target   `yaml:"defaults"`
	Targets       []Target `yaml:"targets"`
}

// TLSSrv permet d'exposer /metrics en HTTPS (optionnel).
type TLSSrv struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Target décrit un vCenter (8.x ou 9.x / VCF 9) à superviser.
type Target struct {
	Name               string        `yaml:"name"`
	URL                string        `yaml:"url"`
	Username           string        `yaml:"username"`
	UsernameFile       string        `yaml:"username_file"`
	Password           string        `yaml:"password"`
	PasswordFile       string        `yaml:"password_file"`
	PasswordEnv        string        `yaml:"password_env"`
	InsecureSkipVerify *bool         `yaml:"insecure_skip_verify"`
	CAFile             string        `yaml:"ca_file"`
	TLSMinVersion      string        `yaml:"tls_min_version"`
	Interval           time.Duration `yaml:"interval"`
	Timeout            time.Duration `yaml:"timeout"`
	// ForceAPIVersion force la version vim25 (ex "8.0.3.0"). Vide = négociation auto.
	ForceAPIVersion string `yaml:"force_api_version"`

	Collectors Collectors `yaml:"collectors"`
	Filters    Filters    `yaml:"filters"`
	Perf       Perf       `yaml:"perf"`
	Host       HostOpts   `yaml:"host"`
	// Labels statiques ajoutés à toutes les séries de ce vCenter (ex: site, env).
	ExtraLabels map[string]string `yaml:"extra_labels"`
}

// Collectors active / désactive chaque famille de métriques.
type Collectors struct {
	VCenter      *bool `yaml:"vcenter"`
	Alarms       *bool `yaml:"alarms"`
	Datacenter   *bool `yaml:"datacenter"`
	Cluster      *bool `yaml:"cluster"`
	Host         *bool `yaml:"host"`
	VM           *bool `yaml:"vm"`
	Datastore    *bool `yaml:"datastore"`
	Network      *bool `yaml:"network"`
	ResourcePool *bool `yaml:"resourcepool"`
	Perf         *bool `yaml:"perf"`
	VSAN         *bool `yaml:"vsan"`
	Appliance    *bool `yaml:"appliance"`
}

// Filters limite le périmètre (regex sur les noms).
type Filters struct {
	VMExclude        string `yaml:"vm_exclude"`
	VMInclude        string `yaml:"vm_include"`
	HostExclude      string `yaml:"host_exclude"`
	DatastoreExclude string `yaml:"datastore_exclude"`
	// ExcludeSystemVMs exclut les VMs gérées par une solution (vCLS, Supervisor, agents EAM...).
	ExcludeSystemVMs *bool `yaml:"exclude_system_vms"`
	IncludeTemplates *bool `yaml:"include_templates"`
}

// Perf configure la collecte des compteurs de performance temps réel (20s).
type Perf struct {
	HostCounters   []string `yaml:"host_counters"`
	VMCounters     []string `yaml:"vm_counters"`
	BatchSize      int      `yaml:"batch_size"`
	MaxConcurrency int      `yaml:"max_concurrency"`
}

// HostOpts options spécifiques ESX.
type HostOpts struct {
	StoragePaths *bool    `yaml:"storage_paths"`
	Sensors      *bool    `yaml:"sensors"`
	Services     []string `yaml:"services"`
}

// reservedLabels : labels émis par l'exporter (ou ajoutés par Prometheus) qu'un extra_labels ne doit pas masquer.
var reservedLabels = map[string]bool{}

func init() {
	for _, l := range strings.Fields(`acknowledged alarm alarm_moid api_version_server api_version_used bios_version build cluster collector commit component cpu_model datacenter datastore datastore_cluster datastore_moid disk_path driver dvs endpoint entity entity_moid entity_type govmomi_api group guest_hostname guest_os host hw_version instance instance_uuid ip_address job managed_by model name network network_moid network_type nic perf_instance policy product product_line resource_pool resource_pool_moid sensor service shared startup_type state status template test tools_version type unit url used vcenter vendor version vm vm_moid`) {
		reservedLabels[l] = true
	}
}

var envRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load lit le fichier, interpole ${VAR}, applique les valeurs par défaut et valide.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lecture %s: %w", path, err)
	}
	return Parse(raw)
}

// Parse analyse un contenu YAML.
func Parse(raw []byte) (*Config, error) {
	expanded := envRe.ReplaceAllStringFunc(string(raw), func(m string) string {
		return os.Getenv(envRe.FindStringSubmatch(m)[1])
	})
	cfg := &Config{}
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("YAML invalide: %w", err)
	}
	if cfg.ListenAddress == "" {
		cfg.ListenAddress = ":9272"
	}
	if cfg.TelemetryPath == "" {
		cfg.TelemetryPath = "/metrics"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("aucune cible (targets) définie")
	}
	seen := map[string]bool{}
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		mergeDefaults(t, &cfg.Defaults)
		if err := t.finalize(); err != nil {
			return nil, fmt.Errorf("cible #%d (%s): %w", i, t.Name, err)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("nom de cible dupliqué: %s", t.Name)
		}
		seen[t.Name] = true
	}
	// Même jeu de labels additionnels sur toutes les cibles (valeur vide si absent),
	// pour garder des familles de métriques homogènes.
	union := map[string]bool{}
	for _, t := range cfg.Targets {
		for k := range t.ExtraLabels {
			union[k] = true
		}
	}
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		for k := range union {
			if reservedLabels[k] {
				return nil, fmt.Errorf("cible %s: extra_labels %q est réservé par l'exporter", t.Name, k)
			}
			if t.ExtraLabels == nil {
				t.ExtraLabels = map[string]string{}
			}
			if _, ok := t.ExtraLabels[k]; !ok {
				t.ExtraLabels[k] = ""
			}
		}
	}
	return cfg, nil
}

func b(v bool) *bool { return &v }

func mergeDefaults(t, d *Target) {
	str := func(dst *string, src string) {
		if *dst == "" {
			*dst = src
		}
	}
	str(&t.Username, d.Username)
	str(&t.UsernameFile, d.UsernameFile)
	str(&t.Password, d.Password)
	str(&t.PasswordFile, d.PasswordFile)
	str(&t.PasswordEnv, d.PasswordEnv)
	str(&t.CAFile, d.CAFile)
	str(&t.TLSMinVersion, d.TLSMinVersion)
	str(&t.ForceAPIVersion, d.ForceAPIVersion)
	if t.InsecureSkipVerify == nil {
		t.InsecureSkipVerify = d.InsecureSkipVerify
	}
	if t.Interval == 0 {
		t.Interval = d.Interval
	}
	if t.Timeout == 0 {
		t.Timeout = d.Timeout
	}
	bp := func(dst **bool, src *bool) {
		if *dst == nil {
			*dst = src
		}
	}
	c, dc := &t.Collectors, &d.Collectors
	bp(&c.VCenter, dc.VCenter)
	bp(&c.Alarms, dc.Alarms)
	bp(&c.Datacenter, dc.Datacenter)
	bp(&c.Cluster, dc.Cluster)
	bp(&c.Host, dc.Host)
	bp(&c.VM, dc.VM)
	bp(&c.Datastore, dc.Datastore)
	bp(&c.Network, dc.Network)
	bp(&c.ResourcePool, dc.ResourcePool)
	bp(&c.Perf, dc.Perf)
	bp(&c.VSAN, dc.VSAN)
	bp(&c.Appliance, dc.Appliance)
	f, df := &t.Filters, &d.Filters
	str(&f.VMExclude, df.VMExclude)
	str(&f.VMInclude, df.VMInclude)
	str(&f.HostExclude, df.HostExclude)
	str(&f.DatastoreExclude, df.DatastoreExclude)
	bp(&f.ExcludeSystemVMs, df.ExcludeSystemVMs)
	bp(&f.IncludeTemplates, df.IncludeTemplates)
	if len(t.Perf.HostCounters) == 0 {
		t.Perf.HostCounters = d.Perf.HostCounters
	}
	if len(t.Perf.VMCounters) == 0 {
		t.Perf.VMCounters = d.Perf.VMCounters
	}
	if t.Perf.BatchSize == 0 {
		t.Perf.BatchSize = d.Perf.BatchSize
	}
	if t.Perf.MaxConcurrency == 0 {
		t.Perf.MaxConcurrency = d.Perf.MaxConcurrency
	}
	bp(&t.Host.StoragePaths, d.Host.StoragePaths)
	bp(&t.Host.Sensors, d.Host.Sensors)
	if len(t.Host.Services) == 0 {
		t.Host.Services = d.Host.Services
	}
	if len(d.ExtraLabels) > 0 {
		if t.ExtraLabels == nil {
			t.ExtraLabels = map[string]string{}
		}
		for k, v := range d.ExtraLabels {
			if _, ok := t.ExtraLabels[k]; !ok {
				t.ExtraLabels[k] = v
			}
		}
	}
}

// DefaultHostCounters compteurs temps réel ESX collectés par défaut.
// Le suffixe ":*" demande toutes les instances (sinon agrégat "" uniquement).
var DefaultHostCounters = []string{
	"cpu.usage.average",
	"cpu.utilization.average",
	"cpu.coreUtilization.average",
	"mem.usage.average",
	"mem.active.average",
	"mem.consumed.average",
	"mem.vmmemctl.average",
	"mem.swapinRate.average",
	"mem.swapoutRate.average",
	"disk.maxTotalLatency.latest",
	"disk.read.average",
	"disk.write.average",
	"disk.commandsAborted.summation",
	"disk.busResets.summation",
	"net.received.average",
	"net.transmitted.average",
	"net.droppedRx.summation",
	"net.droppedTx.summation",
	"net.errorsRx.summation",
	"net.errorsTx.summation",
	"power.power.average",
	"datastore.totalReadLatency.average:*",
	"datastore.totalWriteLatency.average:*",
}

// DefaultVMCounters compteurs temps réel VM collectés par défaut.
var DefaultVMCounters = []string{
	"cpu.usage.average",
	"cpu.ready.summation",
	"cpu.costop.summation",
	"cpu.latency.average",
	"cpu.readiness.average",
	"mem.usage.average",
	"mem.active.average",
	"mem.vmmemctl.average",
	"mem.swapped.average",
	"mem.swapinRate.average",
	"disk.maxTotalLatency.latest",
	"disk.read.average",
	"disk.write.average",
	"net.received.average",
	"net.transmitted.average",
	"net.droppedRx.summation",
	"net.droppedTx.summation",
}

func (t *Target) finalize() error {
	if t.Name == "" {
		return fmt.Errorf("champ 'name' obligatoire")
	}
	if t.URL == "" {
		return fmt.Errorf("champ 'url' obligatoire")
	}
	u, err := ParseVCURL(t.URL)
	if err != nil {
		return err
	}
	t.URL = u.String()
	if t.UsernameFile != "" {
		v, err := readSecret(t.UsernameFile)
		if err != nil {
			return err
		}
		t.Username = v
	}
	if t.Username == "" {
		return fmt.Errorf("identifiant manquant (username ou username_file)")
	}
	switch {
	case t.PasswordFile != "":
		v, err := readSecret(t.PasswordFile)
		if err != nil {
			return err
		}
		t.Password = v
	case t.PasswordEnv != "":
		t.Password = os.Getenv(t.PasswordEnv)
	}
	if t.Password == "" {
		return fmt.Errorf("mot de passe manquant (password_file, password_env ou password)")
	}
	if t.Interval == 0 {
		t.Interval = 60 * time.Second
	}
	if t.Interval < 20*time.Second {
		return fmt.Errorf("interval minimal 20s (granularité temps réel vSphere)")
	}
	if t.Timeout == 0 {
		t.Timeout = t.Interval - 5*time.Second
	}
	if t.Timeout > t.Interval {
		t.Timeout = t.Interval
	}
	if t.InsecureSkipVerify == nil {
		t.InsecureSkipVerify = b(false)
	}
	if t.TLSMinVersion == "" {
		t.TLSMinVersion = "1.2"
	}
	if t.TLSMinVersion != "1.2" && t.TLSMinVersion != "1.3" {
		return fmt.Errorf("tls_min_version doit valoir 1.2 ou 1.3")
	}
	on := func(p **bool, def bool) {
		if *p == nil {
			*p = b(def)
		}
	}
	c := &t.Collectors
	on(&c.VCenter, true)
	on(&c.Alarms, true)
	on(&c.Datacenter, true)
	on(&c.Cluster, true)
	on(&c.Host, true)
	on(&c.VM, true)
	on(&c.Datastore, true)
	on(&c.Network, true)
	on(&c.ResourcePool, true)
	on(&c.Perf, true)
	on(&c.VSAN, true)
	on(&c.Appliance, true)
	on(&t.Filters.ExcludeSystemVMs, false)
	on(&t.Filters.IncludeTemplates, false)
	on(&t.Host.StoragePaths, true)
	on(&t.Host.Sensors, true)
	if len(t.Host.Services) == 0 {
		t.Host.Services = []string{"TSM-SSH", "TSM", "ntpd", "chronyd", "vpxa", "hostd"}
	}
	for _, re := range []string{t.Filters.VMExclude, t.Filters.VMInclude, t.Filters.HostExclude, t.Filters.DatastoreExclude} {
		if re != "" {
			if _, err := regexp.Compile(re); err != nil {
				return fmt.Errorf("regex invalide %q: %w", re, err)
			}
		}
	}
	if len(t.Perf.HostCounters) == 0 {
		t.Perf.HostCounters = DefaultHostCounters
	}
	if len(t.Perf.VMCounters) == 0 {
		t.Perf.VMCounters = DefaultVMCounters
	}
	if t.Perf.BatchSize <= 0 {
		t.Perf.BatchSize = 100
	}
	if t.Perf.MaxConcurrency <= 0 {
		t.Perf.MaxConcurrency = 4
	}
	for k := range t.ExtraLabels {
		if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(k) {
			return fmt.Errorf("extra_labels: nom de label invalide %q", k)
		}
	}
	return nil
}

// ParseVCURL accepte "vcenter.local", "https://vcenter.local" ou ".../sdk" et normalise en https://host/sdk.
func ParseVCURL(s string) (*url.URL, error) {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("url invalide: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("url invalide: hôte vide")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/sdk"
	}
	u.User = nil
	return u, nil
}

func readSecret(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lecture du secret %s: %w", path, err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}
