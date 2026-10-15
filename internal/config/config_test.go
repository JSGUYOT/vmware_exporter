package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDefaultsAndSecrets(t *testing.T) {
	dir := t.TempDir()
	pf := filepath.Join(dir, "pass")
	if err := os.WriteFile(pf, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VC_USER", "svc-monitoring@vsphere.local")
	t.Setenv("VC2_PASS", "fromenv")
	cfg, err := Parse([]byte(`
defaults:
  username: ${VC_USER}
  password_file: ` + pf + `
  interval: 2m
  collectors: {vsan: false}
  extra_labels: {env: prod}
targets:
  - name: vc8
    url: vcenter8.example.local
  - name: vcf9
    url: https://vcenter9.example.local/sdk
    password_env: VC2_PASS
    password_file: ""
    collectors: {vsan: true}
    extra_labels: {site: b}
`))
	if err != nil {
		t.Fatal(err)
	}
	a, b := cfg.Targets[0], cfg.Targets[1]
	if a.URL != "https://vcenter8.example.local/sdk" || a.Username != "svc-monitoring@vsphere.local" || a.Password != "s3cret" {
		t.Errorf("cible 1 mal normalisée: %+v", a)
	}
	if a.Interval != 2*time.Minute || a.Timeout != 2*time.Minute-5*time.Second {
		t.Errorf("intervalle/timeout: %v %v", a.Interval, a.Timeout)
	}
	if *a.Collectors.VSAN || !*b.Collectors.VSAN || !*a.Collectors.Alarms {
		t.Errorf("héritage des collecteurs incorrect")
	}
	if a.ExtraLabels["site"] != "" || b.ExtraLabels["env"] != "prod" || b.ExtraLabels["site"] != "b" {
		t.Errorf("extra_labels: %v / %v", a.ExtraLabels, b.ExtraLabels)
	}
	if _, ok := a.ExtraLabels["site"]; !ok {
		t.Errorf("label site non homogénéisé")
	}
	if len(a.Perf.VMCounters) == 0 || len(a.Host.Services) == 0 {
		t.Errorf("valeurs par défaut perf/services manquantes")
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"sans cible":    `listen_address: ":1"`,
		"sans mdp":      "targets: [{name: a, url: x, username: u}]",
		"intervalle":    "targets: [{name: a, url: x, username: u, password: p, interval: 5s}]",
		"doublon":       "targets: [{name: a, url: x, username: u, password: p}, {name: a, url: y, username: u, password: p}]",
		"label réservé": "targets: [{name: a, url: x, username: u, password: p, extra_labels: {host: z}}]",
		"champ inconnu": "targets: [{name: a, url: x, username: u, password: p, passwd: z}]",
		"regex":         "targets: [{name: a, url: x, username: u, password: p, filters: {vm_exclude: '('}}]",
	}
	for name, y := range cases {
		if _, err := Parse([]byte(y)); err == nil {
			t.Errorf("%s: erreur attendue", name)
		}
	}
}
