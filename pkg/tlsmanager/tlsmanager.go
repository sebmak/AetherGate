package tlsmanager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/store"
	"golang.org/x/crypto/acme/autocert"
)

// Manager handles TLS certificate issuance, caching, and host policies
type Manager struct {
	mu             sync.RWMutex
	store          *store.Store
	certDir        string
	autocertMgr    *autocert.Manager
	selfSignedCert *tls.Certificate
}

// NewManager creates a TLS certificate Manager
func NewManager(s *store.Store, certDir string) (*Manager, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert dir: %w", err)
	}

	m := &Manager{
		store:   s,
		certDir: certDir,
	}

	// Host policy dynamically verifies that the incoming SNI hostname corresponds to an active proxy route
	hostPolicy := func(ctx context.Context, host string) error {
		// Clean hostname
		cleanHost := strings.ToLower(strings.TrimSpace(host))
		if _, found := m.store.GetTargetByDomain(cleanHost); found {
			return nil
		}
		return fmt.Errorf("acme/autocert: host %s is not a registered proxy target", host)
	}

	tlsCfg := s.GetTLSConfig()
	email := strings.TrimSpace(tlsCfg.Email)
	if store.IsForbiddenEmail(email) {
		email = ""
	}

	m.autocertMgr = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Email:      email,
		Cache:      autocert.DirCache(certDir),
	}

	return m, nil
}

// GetTLSConfig returns a tls.Config configured for dynamic certificate retrieval
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1", "acme-tls/1"},
	}
}

// GetCertificate returns a certificate for the client handshake
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	tlsCfg := m.store.GetTLSConfig()

	// 1. If custom certificates are provided and exist, use them
	if tlsCfg.Mode == "custom" && tlsCfg.CertPath != "" && tlsCfg.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertPath, tlsCfg.KeyPath)
		if err == nil {
			logger.SystemInfo("TLS", "Served custom TLS certificate for '%s'", hello.ServerName)
			return &cert, nil
		}
		logger.SystemError("TLS", "Failed to load custom TLS cert files (Cert: %s, Key: %s): %v. Falling back to self-signed cert.", tlsCfg.CertPath, tlsCfg.KeyPath, err)
	}

	// 2. If autocert mode is active and we have an SNI server name, request via Let's Encrypt
	if tlsCfg.Mode == "autocert" || tlsCfg.Mode == "" {
		if hello.ServerName != "" {
			cleanHost := strings.ToLower(strings.TrimSpace(hello.ServerName))
			// Only invoke ACME autocert for registered proxy domains
			if _, found := m.store.GetTargetByDomain(cleanHost); found {
				m.mu.Lock()
				currentEmail := strings.TrimSpace(tlsCfg.Email)
				if store.IsForbiddenEmail(currentEmail) {
					currentEmail = ""
				}
				m.autocertMgr.Email = currentEmail
				m.mu.Unlock()

				cert, err := m.autocertMgr.GetCertificate(hello)
				if err == nil {
					logger.SystemInfo("TLS", "Valid Let's Encrypt certificate retrieved for '%s'", hello.ServerName)
					return cert, nil
				}

				// Diagnose cause of ACME certificate generation failure
				errStr := err.Error()
				reason := ""
				if strings.Contains(errStr, "invalidContact") || strings.Contains(errStr, "forbidden domain") {
					reason = "Registration contact email was rejected by Let's Encrypt (e.g. example.com). Update email in Settings."
				} else if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
					reason = "Let's Encrypt cannot connect to your server from the internet. Verify router forwards port 80/443 to this machine and DNS points to your public IP."
				} else if strings.Contains(errStr, "rate limited") {
					reason = "Let's Encrypt rate limit reached for this domain. Wait before requesting new certificates."
				} else if strings.Contains(errStr, "NXDOMAIN") {
					reason = "Domain does not exist in public DNS. Ensure Cloudflare DDNS has synced your public IP."
				} else if strings.Contains(errStr, "unauthorized") {
					reason = "ACME HTTP-01 challenge unauthorized. Ensure incoming traffic on port 80 reaches ProxyServer without being blocked or redirected."
				}

				if reason != "" {
					logger.SystemError("TLS", "ACME certificate request for '%s' failed: %v | Cause: %s (Falling back to dev cert)", hello.ServerName, err, reason)
				} else {
					logger.SystemError("TLS", "ACME certificate request for '%s' failed: %v (Falling back to dev cert)", hello.ServerName, err)
				}
			} else {
				logger.SystemWarn("TLS", "Incoming SNI '%s' is not in registered proxy targets. ACME skipped, serving fallback dev cert.", hello.ServerName)
			}
		} else {
			logger.SystemWarn("TLS", "Incoming TLS handshake without SNI hostname. Serving fallback dev cert.")
		}
	}

	// 3. Fallback to self-signed certificate for local dev / unconfigured hostnames
	return m.getOrCreateSelfSigned()
}

// HTTPHandler wraps an HTTP handler to intercept ACME challenges
func (m *Manager) HTTPHandler(fallback http.Handler) http.Handler {
	acmeHandler := m.autocertMgr.HTTPHandler(fallback)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			logger.SystemInfo("TLS", "Received Let's Encrypt ACME HTTP-01 verification challenge for '%s' from %s", r.Host, r.RemoteAddr)
		}
		acmeHandler.ServeHTTP(w, r)
	})
}

func (m *Manager) getOrCreateSelfSigned() (*tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.selfSignedCert != nil {
		return m.selfSignedCert, nil
	}

	// Generate in-memory self-signed certificate
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"ProxyServer Self-Signed Gateway"},
			CommonName:   "localhost",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "*.localhost", "home.local"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	m.selfSignedCert = &cert
	return m.selfSignedCert, nil
}

// UpdateEmail updates the autocert ACME account email
func (m *Manager) UpdateEmail(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autocertMgr.Email = email
}
