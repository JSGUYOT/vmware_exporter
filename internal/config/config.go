// Package config loads and validates the exporter's YAML configuration.
//
// No secret or URL is hard-coded: everything comes from the configuration
// file, environment variables (${VAR} interpolation) or dedicated password
// files.
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

// Config is the root of the configuration file.
type Config struct {
	ListenAddress string  `yaml:"listen_address"`
	TelemetryPath string  `yaml:"telemetry_path"`
	LogLevel      string  `yaml:"log_level"`
	LogFormat     string  `yaml:"log_format"`
	TLSServer     *TLSSrv `yaml:"web_tls"`
	// WebConfigFile protects the HTTP endpoints (TLS, basic authentication)
	// with a Prometheus web configuration file; empty = public endpoints.
	WebConfigFile string   `yaml:"web_config_file"`
	Defaults      Target   `yaml:"defaults"`
	Targets       []Target `yaml:"targets"`
}

// TLSSrv serves /metrics over HTTPS (optional).
type TLSSrv struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// Target describes a vCenter (8.x or 9.x / VCF 9) to monitor.
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
	// ForceAPIVersion pins the vim25 version (e.g. "8.0.3.0"). Empty = auto-negotiation.
	ForceAPIVersion string `yaml:"force_api_version"`

	Collectors Collectors `yaml:"collectors"`
	Filters    Filters    `yaml:"filters"`
	Perf       Perf       `yaml:"perf"`
	Host       HostOpts   `yaml:"host"`
	// Static labels added to every series of this vCenter (e.g. site, env).
	ExtraLabels map[string]string `yaml:"extra_labels"`
}

// Collectors enables / disables each metric family.
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
	// Pryorda emits the metric names and labels of pryorda/vmware_exporter
	// (in addition to the native ones), for existing dashboards.
	Pryorda *bool `yaml:"pryorda"`
}

// Filters narrows the scope (regexes on names).
type Filters struct {
	VMExclude        string `yaml:"vm_exclude"`
	VMInclude        string `yaml:"vm_include"`
	HostExclude      string `yaml:"host_exclude"`
	DatastoreExclude string `yaml:"datastore_exclude"`
	// ExcludeSystemVMs excludes solution-managed VMs (vCLS, Supervisor, EAM agents...).
	ExcludeSystemVMs *bool `yaml:"exclude_system_vms"`
	IncludeTemplates *bool `yaml:"include_templates"`
}

// Perf configures collection of real-time (20s) performance counters.
type Perf struct {
	HostCounters   []string `yaml:"host_counters"`
	VMCounters     []string `yaml:"vm_counters"`
	BatchSize      int      `yaml:"batch_size"`
	MaxConcurrency int      `yaml:"max_concurrency"`
}

// HostOpts holds ESX-specific options.
type HostOpts struct {
	StoragePaths *bool    `yaml:"storage_paths"`
	Sensors      *bool    `yaml:"sensors"`
	Services     []string `yaml:"services"`
}

// reservedLabels: labels emitted by the exporter (or added by Prometheus) that extra_labels must not shadow.
var reservedLabels = map[string]bool{}

func init() {
	for _, l := range strings.Fields(`acknowledged alarm alarm_moid api_version_server api_version_used bios_version build cluster_name collector commit component cpu_model dc_name ds_name datastore_cluster datastore_moid disk_path driver dvs endpoint entity entity_moid entity_type folder_name govmomi_api group guest_hostname guest_os host_name hw_version instance instance_uuid ip_address job managed_by model name network network_moid network_type nic perf_instance policy product product_line resource_pool resource_pool_moid sensor service shared startup_type state status template test tools_version type unit url used vcenter vendor version vm_moid vm_name`) {
		reservedLabels[l] = true
	}
}

var envRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Load reads the file, interpolates ${VAR}, applies defaults and validates.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(raw)
}

// Parse parses YAML content.
func Parse(raw []byte) (*Config, error) {
	expanded := envRe.ReplaceAllStringFunc(string(raw), func(m string) string {
		return os.Getenv(envRe.FindStringSubmatch(m)[1])
	})
	cfg := &Config{}
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
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
	if cfg.WebConfigFile != "" && cfg.TLSServer != nil && cfg.TLSServer.CertFile != "" {
		return nil, fmt.Errorf("web_tls and web_config_file are mutually exclusive: set tls_server_config in the web configuration file")
	}
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("no targets defined")
	}
	seen := map[string]bool{}
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		mergeDefaults(t, &cfg.Defaults)
		if err := t.finalize(); err != nil {
			return nil, fmt.Errorf("target #%d (%s): %w", i, t.Name, err)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("duplicate target name: %s", t.Name)
		}
		seen[t.Name] = true
	}
	// Same set of extra labels on every target (empty value if missing),
	// so that metric families stay consistent.
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
				return nil, fmt.Errorf("target %s: extra_labels %q is reserved by the exporter", t.Name, k)
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
	bp(&c.Pryorda, dc.Pryorda)
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

// DefaultHostCounters are the ESX real-time counters collected by default.
// The ":*" suffix requests all instances (otherwise only the "" aggregate).
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
	"datastore.read.average:*",
	"datastore.write.average:*",
	"datastore.numberReadAveraged.average:*",
	"datastore.numberWriteAveraged.average:*",
}

// DefaultVMCounters are the VM real-time counters collected by default.
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
		return fmt.Errorf("field 'name' is required")
	}
	if t.URL == "" {
		return fmt.Errorf("field 'url' is required")
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
		return fmt.Errorf("missing username (username or username_file)")
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
		return fmt.Errorf("missing password (password_file, password_env or password)")
	}
	if t.Interval == 0 {
		t.Interval = 60 * time.Second
	}
	if t.Interval < 20*time.Second {
		return fmt.Errorf("minimum interval is 20s (vSphere real-time granularity)")
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
		return fmt.Errorf("tls_min_version must be 1.2 or 1.3")
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
	on(&c.Pryorda, false)
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
				return fmt.Errorf("invalid regex %q: %w", re, err)
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
			return fmt.Errorf("extra_labels: invalid label name %q", k)
		}
	}
	return nil
}

// ParseVCURL accepts "vcenter.local", "https://vcenter.local" or ".../sdk" and normalizes to https://host/sdk.
func ParseVCURL(s string) (*url.URL, error) {
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid url: empty host")
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
		return "", fmt.Errorf("reading secret %s: %w", path, err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}
