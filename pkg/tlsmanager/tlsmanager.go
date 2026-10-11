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
	email := tlsCfg.Email
	if email == "" {
		email = "admin@example.com"
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
			return &cert, nil
		}
		logger.Warn("Failed to load custom TLS cert (%s, %s): %v", tlsCfg.CertPath, tlsCfg.KeyPath, err)
	}

	// 2. If autocert mode is active and we have an SNI server name, request via Let's Encrypt
	if tlsCfg.Mode == "autocert" || tlsCfg.Mode == "" {
		if hello.ServerName != "" {
			cert, err := m.autocertMgr.GetCertificate(hello)
			if err == nil {
				return cert, nil
			}
			logger.Debug("Autocert certificate lookup for %s returned: %v (falling back to dev cert)", hello.ServerName, err)
		}
	}

	// 3. Fallback to self-signed certificate for local dev / unconfigured hostnames
	return m.getOrCreateSelfSigned()
}

// HTTPHandler wraps an HTTP handler to intercept ACME challenges
func (m *Manager) HTTPHandler(fallback http.Handler) http.Handler {
	return m.autocertMgr.HTTPHandler(fallback)
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
