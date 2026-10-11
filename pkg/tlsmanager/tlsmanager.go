package tlsmanager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/store"
	"golang.org/x/crypto/acme/autocert"
)

// CertificateInfo represents the SSL/TLS certificate health, validity, and metadata for a domain
type CertificateInfo struct {
	Domain        string     `json:"domain"`
	Status        string     `json:"status"` // "valid", "expiring_soon", "expired", "self_signed", "error", "renewing"
	IsValid       bool       `json:"is_valid"`
	IsSelfSigned  bool       `json:"is_self_signed"`
	Issuer        string     `json:"issuer,omitempty"`
	Subject       string     `json:"subject,omitempty"`
	IssuedAt      *time.Time `json:"issued_at,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	DaysRemaining int        `json:"days_remaining"`
	LastError     string     `json:"last_error,omitempty"`
	Mode          string     `json:"mode"`
	Serial        string     `json:"serial,omitempty"`
	DNSNames      []string   `json:"dns_names,omitempty"`
}

// Manager handles TLS certificate issuance, caching, renewal, and host policies.
// All certificate lifecycle events and ACME handshakes are coordinated thread-safely through this manager.
type Manager struct {
	mu             sync.RWMutex
	store          *store.Store
	certDir        string
	autocertMgr    *autocert.Manager
	selfSignedCert *tls.Certificate
	lastErrors     map[string]string
	renewing       map[string]bool
}

// NewManager creates a TLS certificate Manager
func NewManager(s *store.Store, certDir string) (*Manager, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert dir: %w", err)
	}

	m := &Manager{
		store:      s,
		certDir:    certDir,
		lastErrors: make(map[string]string),
		renewing:   make(map[string]bool),
	}

	m.initAutocertManager()
	return m, nil
}

func (m *Manager) initAutocertManager() {
	// Host policy dynamically verifies that the incoming SNI hostname corresponds to an active proxy route
	hostPolicy := func(ctx context.Context, host string) error {
		cleanHost := strings.ToLower(strings.TrimSpace(host))
		if _, found := m.store.GetTargetByDomain(cleanHost); found {
			return nil
		}
		return fmt.Errorf("acme/autocert: host %s is not a registered proxy target", host)
	}

	tlsCfg := m.store.GetTLSConfig()
	email := strings.TrimSpace(tlsCfg.Email)
	if store.IsForbiddenEmail(email) {
		email = ""
	}

	m.autocertMgr = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Email:      email,
		Cache:      autocert.DirCache(m.certDir),
	}
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
				mgr := m.autocertMgr
				m.mu.Unlock()

				cert, err := mgr.GetCertificate(hello)
				if err == nil {
					m.mu.Lock()
					delete(m.lastErrors, cleanHost)
					m.mu.Unlock()
					logger.SystemInfo("TLS", "Valid Let's Encrypt certificate retrieved for '%s'", hello.ServerName)
					return cert, nil
				}

				// Diagnose cause of ACME certificate generation failure
				diag := diagnoseACMEError(err)
				m.mu.Lock()
				m.lastErrors[cleanHost] = diag
				m.mu.Unlock()
				logger.SystemError("TLS", "ACME certificate request for '%s' failed: %s (Falling back to dev cert)", hello.ServerName, diag)
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			logger.SystemInfo("TLS", "Received Let's Encrypt ACME HTTP-01 verification challenge for '%s' from %s", r.Host, r.RemoteAddr)
		}
		m.mu.RLock()
		am := m.autocertMgr
		m.mu.RUnlock()
		if am != nil {
			am.HTTPHandler(fallback).ServeHTTP(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}

// GetCertificateInfo inspects the active certificate status, validity, and metadata for a domain
func (m *Manager) GetCertificateInfo(domain string) *CertificateInfo {
	cleanHost := strings.ToLower(strings.TrimSpace(domain))
	tlsCfg := m.store.GetTLSConfig()

	m.mu.RLock()
	lastErr := m.lastErrors[cleanHost]
	isRenewing := m.renewing[cleanHost]
	m.mu.RUnlock()

	mode := tlsCfg.Mode
	if mode == "" {
		mode = "autocert"
	}

	info := &CertificateInfo{
		Domain:    cleanHost,
		Mode:      mode,
		LastError: lastErr,
	}

	if isRenewing {
		info.Status = "renewing"
		info.IsSelfSigned = true
		return info
	}

	// 1. Custom Certificate Mode
	if mode == "custom" && tlsCfg.CertPath != "" {
		if certData, err := os.ReadFile(tlsCfg.CertPath); err == nil {
			if leaf, err := parseCertFromPEM(certData); err == nil {
				return populateCertInfo(info, leaf, false)
			}
		}
		info.Status = "error"
		info.LastError = "Failed to load custom certificate file"
		return info
	}

	// 2. Autocert Mode: Check if a certificate file is cached on disk
	if mode == "autocert" {
		certPath := filepath.Join(m.certDir, cleanHost)
		if certData, err := os.ReadFile(certPath); err == nil {
			if leaf, err := parseCertFromPEM(certData); err == nil {
				res := populateCertInfo(info, leaf, false)
				if lastErr != "" {
					res.LastError = lastErr
				}
				return res
			}
		}
	}

	// 3. Fallback / Self-signed certificate
	selfSigned, err := m.getOrCreateSelfSigned()
	if err == nil && len(selfSigned.Certificate) > 0 {
		if devCert, err := x509.ParseCertificate(selfSigned.Certificate[0]); err == nil {
			info = populateCertInfo(info, devCert, true)
			if lastErr != "" {
				info.Status = "error"
				info.LastError = lastErr
			} else {
				info.Status = "self_signed"
			}
			return info
		}
	}

	if lastErr != "" {
		info.Status = "error"
	} else {
		info.Status = "self_signed"
	}
	info.IsSelfSigned = true
	return info
}

// GetAllCertificatesInfo returns status for all unique domains in proxy targets
func (m *Manager) GetAllCertificatesInfo() []*CertificateInfo {
	targets := m.store.GetTargets()
	uniqueDomains := make(map[string]bool)
	var results []*CertificateInfo

	for _, t := range targets {
		d := strings.ToLower(strings.TrimSpace(t.Domain))
		if d != "" && !uniqueDomains[d] {
			uniqueDomains[d] = true
			results = append(results, m.GetCertificateInfo(d))
		}
	}
	return results
}

// RenewCertificate forces a renewal for a specific domain by purging cache and calling ACME
func (m *Manager) RenewCertificate(ctx context.Context, domain string) (*CertificateInfo, error) {
	cleanHost := strings.ToLower(strings.TrimSpace(domain))
	if cleanHost == "" {
		return nil, fmt.Errorf("domain name cannot be empty")
	}

	tlsCfg := m.store.GetTLSConfig()
	if tlsCfg.Mode == "custom" {
		return nil, fmt.Errorf("server is configured in Custom Certificate mode; update custom certificate files directly")
	}

	m.mu.Lock()
	if m.renewing[cleanHost] {
		m.mu.Unlock()
		return m.GetCertificateInfo(cleanHost), fmt.Errorf("certificate renewal is already in progress for %s", cleanHost)
	}
	m.renewing[cleanHost] = true
	delete(m.lastErrors, cleanHost)
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.renewing, cleanHost)
		m.mu.Unlock()
	}()

	logger.SystemInfo("TLS", "Manual certificate regeneration initiated for '%s'", cleanHost)

	// Remove cached certificate on disk
	certFile := filepath.Join(m.certDir, cleanHost)
	_ = os.Remove(certFile)

	// Re-initialize autocert manager so in-memory cache is purged
	m.mu.Lock()
	m.initAutocertManager()
	autocertMgr := m.autocertMgr
	m.mu.Unlock()

	// Trigger certificate retrieval with Let's Encrypt
	hello := &tls.ClientHelloInfo{ServerName: cleanHost}
	_, err := autocertMgr.GetCertificate(hello)
	if err != nil {
		diag := diagnoseACMEError(err)
		m.mu.Lock()
		m.lastErrors[cleanHost] = diag
		m.mu.Unlock()
		logger.SystemError("TLS", "Certificate regeneration failed for '%s': %s", cleanHost, diag)
		return m.GetCertificateInfo(cleanHost), fmt.Errorf("%s", diag)
	}

	m.mu.Lock()
	delete(m.lastErrors, cleanHost)
	m.mu.Unlock()

	logger.SystemInfo("TLS", "Successfully generated and cached new Let's Encrypt certificate for '%s'", cleanHost)
	return m.GetCertificateInfo(cleanHost), nil
}

// RenewAllCertificates forces renewal for all registered domains
func (m *Manager) RenewAllCertificates(ctx context.Context) ([]*CertificateInfo, error) {
	targets := m.store.GetTargets()
	uniqueDomains := make(map[string]bool)
	for _, t := range targets {
		d := strings.ToLower(strings.TrimSpace(t.Domain))
		if d != "" && t.Enabled {
			uniqueDomains[d] = true
		}
	}

	var results []*CertificateInfo
	var lastErr error
	for d := range uniqueDomains {
		info, err := m.RenewCertificate(ctx, d)
		if err != nil {
			lastErr = err
		}
		if info != nil {
			results = append(results, info)
		}
	}
	return results, lastErr
}

func populateCertInfo(info *CertificateInfo, cert *x509.Certificate, isSelfSigned bool) *CertificateInfo {
	now := time.Now()
	issuedAt := cert.NotBefore
	expiresAt := cert.NotAfter
	daysRemaining := int(time.Until(expiresAt).Hours() / 24)

	info.IssuedAt = &issuedAt
	info.ExpiresAt = &expiresAt
	info.DaysRemaining = daysRemaining
	info.IsSelfSigned = isSelfSigned
	info.Subject = cert.Subject.CommonName
	info.DNSNames = cert.DNSNames
	if cert.SerialNumber != nil {
		info.Serial = cert.SerialNumber.Text(16)
	}

	issuer := cert.Issuer.CommonName
	if len(cert.Issuer.Organization) > 0 {
		if issuer != "" && issuer != cert.Issuer.Organization[0] {
			issuer = fmt.Sprintf("%s (%s)", cert.Issuer.Organization[0], issuer)
		} else {
			issuer = cert.Issuer.Organization[0]
		}
	}
	if issuer == "" {
		if isSelfSigned {
			issuer = "ProxyServer Self-Signed Gateway"
		} else {
			issuer = "Let's Encrypt"
		}
	}
	info.Issuer = issuer

	if isSelfSigned {
		info.Status = "self_signed"
		info.IsValid = true
	} else if now.After(expiresAt) {
		info.Status = "expired"
		info.IsValid = false
	} else if daysRemaining <= 15 {
		info.Status = "expiring_soon"
		info.IsValid = true
	} else {
		info.Status = "valid"
		info.IsValid = true
	}

	return info
}

func parseCertFromPEM(data []byte) (*x509.Certificate, error) {
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				return cert, nil
			}
		}
		data = rest
	}
	return nil, fmt.Errorf("no valid CERTIFICATE block found in PEM")
}

func diagnoseACMEError(err error) string {
	errStr := err.Error()
	reason := ""
	if strings.Contains(errStr, "invalidContact") || strings.Contains(errStr, "forbidden domain") {
		reason = "Registration contact email was rejected by Let's Encrypt (e.g. example.com). Update email in Settings."
	} else if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
		reason = "Let's Encrypt could not connect to your server from the internet. Verify router forwards port 80/443 and DNS points to public IP."
	} else if strings.Contains(errStr, "rate limited") {
		reason = "Let's Encrypt rate limit reached for this domain. Wait before requesting new certificates."
	} else if strings.Contains(errStr, "NXDOMAIN") {
		reason = "Domain does not exist in public DNS. Ensure Cloudflare DDNS has synced your public IP."
	} else if strings.Contains(errStr, "unauthorized") {
		reason = "ACME HTTP-01 challenge unauthorized. Ensure incoming traffic on port 80 reaches ProxyServer without being blocked or redirected."
	}

	if reason != "" {
		return fmt.Sprintf("%v | Cause: %s", err, reason)
	}
	return errStr
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

