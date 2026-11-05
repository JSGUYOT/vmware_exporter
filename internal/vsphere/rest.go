package vsphere

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/JSGUYOT/vmware_exporter/internal/config"
)

// ErrNotFound reports an endpoint missing on this vCenter version (or an object deleted meanwhile).
var ErrNotFound = errors.New("REST endpoint not available on this version")

// ErrForbidden reports a privilege missing on the requested object.
var ErrForbidden = errors.New("REST request not allowed for this account")

// RESTClient is a minimal client for the modern vSphere Automation API (/api).
//
// It deliberately uses only /api/... paths (available since 7.0U2, kept in
// 8.x and 9.x) and never the legacy /rest/... prefix, which is deprecated
// and slated for removal in the 9 line.
type RESTClient struct {
	base  *url.URL
	user  string
	pass  string
	http  *http.Client
	mu    sync.Mutex
	token string
}

// NewREST builds the client from the target.
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
		return fmt.Errorf("unexpected /api/session response: %w", err)
	}
	r.token = tok
	return nil
}

// Get performs a JSON GET on an /api/... path, re-authenticating on 401.
func (r *RESTClient) Get(ctx context.Context, path string, out any) error {
	return r.do(ctx, http.MethodGet, path, nil, out)
}

// Post performs a JSON POST on an /api/... path (read-only "?action=" operations),
// re-authenticating on 401.
func (r *RESTClient) Post(ctx context.Context, path string, in, out any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return r.do(ctx, http.MethodPost, path, data, out)
}

func (r *RESTClient) do(ctx context.Context, method, path string, data []byte, out any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if r.token == "" {
			if err := r.login(ctx); err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, r.base.String()+path, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("vmware-api-session-id", r.token)
		req.Header.Set("Accept", "application/json")
		if data != nil {
			req.Header.Set("Content-Type", "application/json")
		}
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
		case res.StatusCode == http.StatusForbidden:
			return fmt.Errorf("%w: %s %s", ErrForbidden, method, path)
		case res.StatusCode >= 300:
			return fmt.Errorf("%s %s: %s %s", method, path, res.Status, strings.TrimSpace(string(body)))
		}
		return json.Unmarshal(body, out)
	}
	return fmt.Errorf("%s %s: REST authentication rejected", method, path)
}

// Logout deletes the REST session.
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
