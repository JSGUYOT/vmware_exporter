// Package vsphere gère la connexion SOAP (govmomi) et REST (/api) à un vCenter
// 8.x ou 9.x (VCF 9), avec négociation de version et reconnexion automatique.
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

	"github.com/example/vmware-exporter/internal/config"
)

// Client encapsule une session vim25 réutilisable entre deux collectes.
type Client struct {
	cfg *config.Target
	log *slog.Logger

	mu   sync.Mutex
	vim  *vim25.Client
	rest *RESTClient

	// Version API vim25 négociée (ex "8.0.3.0", "9.0.0.0").
	APIVersion string
}

// New crée un client non connecté.
func New(cfg *config.Target, log *slog.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// TLSConfig construit la configuration TLS commune SOAP/REST.
func TLSConfig(cfg *config.Target) (*tls.Config, error) {
	tc := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: *cfg.InsecureSkipVerify} //nolint:gosec // choix explicite de l'utilisateur
	if cfg.TLSMinVersion == "1.3" {
		tc.MinVersion = tls.VersionTLS13
	}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("lecture ca_file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %s: aucun certificat PEM valide", cfg.CAFile)
		}
		tc.RootCAs = pool
	}
	return tc, nil
}

// Get renvoie un client vim25 authentifié, en (re)connectant si nécessaire.
func (c *Client) Get(ctx context.Context) (*vim25.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vim != nil {
		// Vérifie que la session est toujours valide (expiration, redémarrage vpxd...).
		us, err := session.NewManager(c.vim).UserSession(ctx)
		switch {
		case err == nil && us != nil:
			return c.vim, nil
		case err != nil && !IsAuthError(err):
			// Erreur transitoire (réseau, timeout) : on garde la session pour ne pas
			// accumuler de sessions orphelines côté vpxd.
			return nil, fmt.Errorf("vérification de session: %w", err)
		}
		c.log.Info("session vCenter expirée ou invalide, reconnexion", "err", err)
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

	// Négociation de version : govmomi parle par défaut la dernière API connue
	// (9.1). Un vCenter 8 doit recevoir sa propre version, d'où la lecture de
	// /sdk/vimServiceVersions.xml (présent en 8.x et 9.x).
	if c.cfg.ForceAPIVersion != "" {
		sc.Version = c.cfg.ForceAPIVersion
	} else if err := sc.UseServiceVersion(); err != nil {
		c.log.Warn("négociation de version impossible, utilisation de la version par défaut govmomi", "err", err, "default", vim25.Version)
	}

	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, fmt.Errorf("connexion %s: %w", u.Host, err)
	}
	if !vc.IsVC() {
		c.log.Warn("la cible n'est pas un vCenter (ESX autonome) : alarmes, clusters et vSAN seront limités")
	}
	if err := session.NewManager(vc).Login(ctx, url.UserPassword(c.cfg.Username, c.cfg.Password)); err != nil {
		return nil, fmt.Errorf("authentification sur %s refusée pour %s: %w", u.Host, c.cfg.Username, err)
	}
	c.APIVersion = sc.Version
	if sc.Version == "" {
		c.APIVersion = vc.ServiceContent.About.ApiVersion
	}
	c.log.Info("connecté au vCenter",
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

// REST renvoie un client REST /api authentifié (sessions séparées de SOAP).
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

// Logout ferme proprement les sessions.
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

// Invalidate force une reconnexion au prochain Get (erreur d'auth détectée).
func (c *Client) Invalidate() {
	c.mu.Lock()
	c.vim = nil
	c.mu.Unlock()
}

// IsAuthError détecte une session expirée côté SOAP.
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

// MajorVersion extrait le majeur d'une version "9.0.0.0" -> 9.
func MajorVersion(v string) int {
	var m int
	_, _ = fmt.Sscanf(v, "%d", &m)
	return m
}

// CertificateExpiry lit la date d'expiration du certificat TLS présenté par le vCenter.
func CertificateExpiry(ctx context.Context, cfg *config.Target) (time.Time, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return time.Time{}, err
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	d := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()}} //nolint:gosec // lecture seule du certificat
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", host)
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	st := conn.(*tls.Conn).ConnectionState()
	if len(st.PeerCertificates) == 0 {
		return time.Time{}, fmt.Errorf("aucun certificat présenté")
	}
	return st.PeerCertificates[0].NotAfter, nil
}
