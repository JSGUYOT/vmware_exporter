// Package vsphere manages the SOAP (govmomi) and REST (/api) connections to a
// vCenter 8.x or 9.x (VCF 9), with version negotiation and automatic reconnection.
package vsphere

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/JSGUYOT/vmware_exporter/internal/config"
)

// Client wraps a vim25 session that is reused across collections.
type Client struct {
	cfg *config.Target
	log *slog.Logger

	mu   sync.Mutex
	vim  *vim25.Client
	rest *RESTClient

	// Negotiated vim25 API version (e.g. "8.0.3.0", "9.0.0.0").
	APIVersion string
}

// New creates an unconnected client.
func New(cfg *config.Target, log *slog.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// TLSConfig builds the TLS configuration shared by SOAP and REST.
func TLSConfig(cfg *config.Target) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: *cfg.InsecureSkipVerify} //nolint:gosec // explicit user choice
	if cfg.TLSMinVersion == "1.3" {
		tc.MinVersion = tls.VersionTLS13
	}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %s: no valid PEM certificate", cfg.CAFile)
		}
		tc.RootCAs = pool
	}
	return tc, nil
}

// Get returns an authenticated vim25 client, (re)connecting if needed.
func (c *Client) Get(ctx context.Context) (*vim25.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vim != nil {
		// Check that the session is still valid (expiry, vpxd restart...).
		us, err := session.NewManager(c.vim).UserSession(ctx)
		switch {
		case err == nil && us != nil:
			return c.vim, nil
		case err != nil && !IsAuthError(err):
			// Transient error (network, timeout): keep the session so as not to
			// pile up orphaned sessions on the vpxd side.
			return nil, fmt.Errorf("session check: %w", err)
		}
		c.log.Info("vCenter session expired or invalid, reconnecting", "err", err)
		c.vim = nil
	}
	vc, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	c.vim = vc
	return vc, nil
}

func (c *Client) connect(ctx context.Context) (*vim25.Client, error) {
	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return nil, err
	}
	tc, err := TLSConfig(c.cfg)
	if err != nil {
		return nil, err
	}
	sc := soap.NewClient(u, *c.cfg.InsecureSkipVerify)
	sc.DefaultTransport().TLSClientConfig = mergeTLS(sc.DefaultTransport().TLSClientConfig, tc)
	sc.Timeout = c.cfg.Timeout

	// Version negotiation: by default govmomi speaks the latest known API
	// (9.1). A vCenter 8 must be sent its own version, hence reading
	// /sdk/vimServiceVersions.xml (present in 8.x and 9.x).
	if c.cfg.ForceAPIVersion != "" {
		sc.Version = c.cfg.ForceAPIVersion
	} else if err := sc.UseServiceVersion(); err != nil {
		c.log.Warn("version negotiation failed, falling back to the govmomi default version", "err", err, "default", vim25.Version)
	}

	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", u.Host, err)
	}
	if !vc.IsVC() {
		c.log.Warn("target is not a vCenter (standalone ESX): alarms, clusters and vSAN will be limited")
	}
	if err := session.NewManager(vc).Login(ctx, url.UserPassword(c.cfg.Username, c.cfg.Password)); err != nil {
		return nil, fmt.Errorf("authentication to %s rejected for %s: %w", u.Host, c.cfg.Username, err)
	}
	c.APIVersion = sc.Version
	if sc.Version == "" {
		c.APIVersion = vc.ServiceContent.About.ApiVersion
	}
	c.log.Info("connected to vCenter",
		"product", vc.ServiceContent.About.FullName,
		"api_version_server", vc.ServiceContent.About.ApiVersion,
		"api_version_used", c.APIVersion)
	return vc, nil
}

func mergeTLS(dst, src *tls.Config) *tls.Config {
	if dst == nil {
		return src
	}
	dst.MinVersion = src.MinVersion
	dst.InsecureSkipVerify = src.InsecureSkipVerify
	if src.RootCAs != nil {
		dst.RootCAs = src.RootCAs
	}
	return dst
}

// REST returns an authenticated /api REST client (sessions separate from SOAP).
func (c *Client) REST(ctx context.Context) (*RESTClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rest == nil {
		rc, err := NewREST(c.cfg)
		if err != nil {
			return nil, err
		}
		c.rest = rc
	}
	return c.rest, nil
}

// Logout cleanly closes the sessions.
func (c *Client) Logout(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vim != nil {
		_ = session.NewManager(c.vim).Logout(ctx)
		c.vim = nil
	}
	if c.rest != nil {
		c.rest.Logout(ctx)
		c.rest = nil
	}
}

// Invalidate forces a reconnection on the next Get (auth error detected).
func (c *Client) Invalidate() {
	c.mu.Lock()
	c.vim = nil
	c.mu.Unlock()
}

// IsAuthError detects an expired SOAP session.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	if soap.IsSoapFault(err) {
		if _, ok := soap.ToSoapFault(err).VimFault().(types.NotAuthenticated); ok {
			return true
		}
	}
	return strings.Contains(err.Error(), "NotAuthenticated")
}

// MajorVersion extracts the major number of a version: "9.0.0.0" -> 9.
func MajorVersion(v string) int {
	var m int
	_, _ = fmt.Sscanf(v, "%d", &m)
	return m
}

// CertificateExpiry reads the expiry date of the TLS certificate presented by the vCenter.
func CertificateExpiry(ctx context.Context, cfg *config.Target) (time.Time, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return time.Time{}, err
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	d := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()}} //nolint:gosec // certificate is only read
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", host)
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	st := conn.(*tls.Conn).ConnectionState()
	if len(st.PeerCertificates) == 0 {
		return time.Time{}, fmt.Errorf("no certificate presented")
	}
	return st.PeerCertificates[0].NotAfter, nil
}
