package vsphere

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/example/vmware-exporter/internal/config"
)

// ErrNotFound indique un endpoint absent sur cette version de vCenter.
var ErrNotFound = errors.New("endpoint REST indisponible sur cette version")

// RESTClient est un client minimal pour l'API vSphere Automation moderne (/api).
//
// On n'utilise volontairement que les chemins /api/... (disponibles depuis
// 7.0U2, conservés en 8.x et 9.x) et jamais l'ancien préfixe /rest/...,
// déprécié et voué à disparaître sur la ligne 9.
type RESTClient struct {
	base  *url.URL
	user  string
	pass  string
	http  *http.Client
	mu    sync.Mutex
	token string
}

// NewREST construit le client à partir de la cible.
func NewREST(cfg *config.Target) (*RESTClient, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, err
	}
	base := &url.URL{Scheme: u.Scheme, Host: u.Host}
	tc, err := TLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tc
	return &RESTClient{
		base: base, user: cfg.Username, pass: cfg.Password,
		http: &http.Client{Transport: tr, Timeout: cfg.Timeout},
	}, nil
}

func (r *RESTClient) login(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base.String()+"/api/session", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(r.user, r.pass)
	res, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /api/session: %s", res.Status)
	}
	var tok string
	if err := json.Unmarshal(body, &tok); err != nil {
		return fmt.Errorf("réponse /api/session inattendue: %w", err)
	}
	r.token = tok
	return nil
}

// Get exécute un GET JSON sur un chemin /api/..., avec reconnexion sur 401.
func (r *RESTClient) Get(ctx context.Context, path string, out any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if r.token == "" {
			if err := r.login(ctx); err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base.String()+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("vmware-api-session-id", r.token)
		req.Header.Set("Accept", "application/json")
		res, err := r.http.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusUnauthorized:
			r.token = ""
			continue
		case res.StatusCode == http.StatusNotFound:
			return ErrNotFound
		case res.StatusCode >= 300:
			return fmt.Errorf("GET %s: %s %s", path, res.Status, strings.TrimSpace(string(body)))
		}
		return json.Unmarshal(body, out)
	}
	return fmt.Errorf("GET %s: authentification REST refusée", path)
}

// Logout supprime la session REST.
func (r *RESTClient) Logout(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.token == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, r.base.String()+"/api/session", nil)
	if err == nil {
		req.Header.Set("vmware-api-session-id", r.token)
		if res, err := r.http.Do(req); err == nil {
			res.Body.Close()
		}
	}
	r.token = ""
}
