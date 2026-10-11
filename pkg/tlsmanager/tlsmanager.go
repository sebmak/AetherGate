package tlsmanager

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/store"
	"golang.org/x/crypto/acme"
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
	mu              sync.RWMutex
	store           *store.Store
	certDir         string
	autocertMgr     *autocert.Manager
	selfSignedCert  *tls.Certificate
	selfSignedCerts map[string]*tls.Certificate
	lastErrors      map[string]string
	renewing        map[string]bool
	httpTokens      map[string]string // challengePath -> challengeResponse
}

// NewManager creates a TLS certificate Manager
func NewManager(s *store.Store, certDir string) (*Manager, error) {
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cert dir: %w", err)
	}

	m := &Manager{
		store:           s,
		certDir:         certDir,
		selfSignedCerts: make(map[string]*tls.Certificate),
		lastErrors:      make(map[string]string),
		renewing:        make(map[string]bool),
		httpTokens:      make(map[string]string),
	}

	m.initAutocertManager()
	return m, nil
}

func (m *Manager) initAutocertManager() {
	// Host policy dynamically verifies that the incoming SNI hostname corresponds to an active proxy route
	hostPolicy := func(ctx context.Context, host string) error {
		cleanHost := strings.ToLower(strings.TrimSpace(host))
		if h, _, err := net.SplitHostPort(cleanHost); err == nil {
			cleanHost = h
		}
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
	if email == "" {
		envEmail := strings.TrimSpace(os.Getenv("EMAIL"))
		if !store.IsForbiddenEmail(envEmail) {
			email = envEmail
		}
	}

	m.autocertMgr = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: hostPolicy,
		Email:      email,
		Cache:      autocert.DirCache(m.certDir),
	}

	// Explicitly invoke HTTPHandler to enable HTTP-01 ACME challenges (sets tryHTTP01 = true)
	_ = m.autocertMgr.HTTPHandler(nil)
}

// GetTLSConfig returns a tls.Config configured for dynamic certificate retrieval
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: m.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
			tls.CurveP384,
		},
		NextProtos: []string{"h2", "http/1.1", "acme-tls/1"},
	}
}

// GetCertificate returns a certificate for the client handshake without blocking on slow ACME challenges
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	tlsCfg := m.store.GetTLSConfig()

	cleanHost := ""
	if hello != nil && hello.ServerName != "" {
		cleanHost = strings.ToLower(strings.TrimSpace(hello.ServerName))
	}

	// 1. If custom certificates are provided and exist, use them
	if tlsCfg.Mode == "custom" && tlsCfg.CertPath != "" && tlsCfg.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertPath, tlsCfg.KeyPath)
		if err == nil {
			logger.SystemInfo("TLS", "Served custom TLS certificate for '%s'", hello.ServerName)
			return &cert, nil
		}
		logger.SystemError("TLS", "Failed to load custom TLS cert files (Cert: %s, Key: %s): %v. Falling back to self-signed cert.", tlsCfg.CertPath, tlsCfg.KeyPath, err)
	}

	// 2. Check if a certificate is already cached on disk (autocert or ACME HTTP-01)
	if cleanHost != "" {
		candidates := []string{
			filepath.Join(m.certDir, cleanHost),
			filepath.Join(m.certDir, cleanHost+"+rsa"),
		}
		for _, certPath := range candidates {
			if certData, err := os.ReadFile(certPath); err == nil {
				if cert, err := tls.X509KeyPair(certData, certData); err == nil {
					return &cert, nil
				}
			}
		}
	}

	// 3. Autocert Mode: if target domain is registered, auto-initiate background ACME issuance
	if (tlsCfg.Mode == "autocert" || tlsCfg.Mode == "") && cleanHost != "" {
		if _, found := m.store.GetTargetByDomain(cleanHost); found {
			m.mu.Lock()
			isRenewing := m.renewing[cleanHost]
			m.mu.Unlock()

			if !isRenewing {
				go func(domain string) {
					logger.SystemInfo("TLS", "Auto-initiating background ACME HTTP-01 challenge for '%s'", domain)
					if _, err := m.RenewCertificate(context.Background(), domain); err != nil {
						logger.SystemWarn("TLS", "Background certificate generation for '%s' completed with error: %v", domain, err)
					} else {
						logger.SystemInfo("TLS", "Background certificate generation for '%s' succeeded!", domain)
					}
				}(cleanHost)
			}
		} else {
			logger.SystemWarn("TLS", "Incoming SNI '%s' is not in registered proxy targets. Serving fallback dev cert.", hello.ServerName)
		}
	} else if cleanHost == "" {
		logger.SystemWarn("TLS", "Incoming TLS handshake without SNI hostname. Serving fallback dev cert.")
	}

	// 4. Return dynamic self-signed certificate matching requested hostname to prevent cipher/SAN mismatches
	return m.getOrCreateSelfSignedForHost(cleanHost)
}

// HTTPHandler wraps an HTTP handler to intercept ACME challenges
func (m *Manager) HTTPHandler(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {
			logger.SystemInfo("TLS", "Received Let's Encrypt ACME HTTP-01 verification challenge for '%s' from %s", r.Host, r.RemoteAddr)

			// 1. Check direct HTTP tokens first
			m.mu.RLock()
			tokenVal, hasToken := m.httpTokens[r.URL.Path]
			m.mu.RUnlock()
			if hasToken {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tokenVal))
				return
			}
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
		candidates := []string{
			filepath.Join(m.certDir, cleanHost),
			filepath.Join(m.certDir, cleanHost+"+rsa"),
		}
		for _, certPath := range candidates {
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
	}

	// 3. Fallback / Self-signed certificate
	selfSigned, err := m.getOrCreateSelfSigned()
	if err == nil && len(selfSigned.Certificate) > 0 {
		if devCert, err := x509.ParseCertificate(selfSigned.Certificate[0]); err == nil {
			info = populateCertInfo(info, devCert, true)
			if lastErr != "" {
				info.Status = "error"
				info.LastError = lastErr
				info.Issuer = "Self-Signed (Let's Encrypt renewal failed)"
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

	// Remove cached certificate files on disk (.pem, +rsa, +token)
	for _, suffix := range []string{"", "+rsa", "+token"} {
		_ = os.Remove(filepath.Join(m.certDir, cleanHost+suffix))
	}

	// Re-initialize autocert manager so in-memory cache is purged
	m.mu.Lock()
	m.initAutocertManager()
	m.mu.Unlock()

	// 1. Direct RFC 8555 HTTP-01 challenge issuance (fast & reliable behind Cloudflare/reverse proxies)
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer httpCancel()

	acmeErr := m.obtainACMEHTTP01Cert(httpCtx, cleanHost)
	if acmeErr != nil {
		diag := diagnoseACMEError(acmeErr)
		m.mu.Lock()
		m.lastErrors[cleanHost] = diag
		delete(m.renewing, cleanHost)
		m.mu.Unlock()
		logger.SystemError("TLS", "Certificate regeneration failed for '%s': %s", cleanHost, diag)
		return m.GetCertificateInfo(cleanHost), fmt.Errorf("%s", diag)
	}

	m.mu.Lock()
	delete(m.lastErrors, cleanHost)
	delete(m.renewing, cleanHost)
	m.initAutocertManager()
	m.mu.Unlock()

	logger.SystemInfo("TLS", "Successfully generated and cached new Let's Encrypt certificate for '%s' via direct HTTP-01", cleanHost)
	return m.GetCertificateInfo(cleanHost), nil
}

// obtainACMEHTTP01Cert performs direct RFC 8555 HTTP-01 challenge issuance via Let's Encrypt
func (m *Manager) obtainACMEHTTP01Cert(ctx context.Context, domain string) error {
	accountKey, err := m.getOrCreateACMEAccountKey()
	if err != nil {
		return fmt.Errorf("failed to load/create ACME account key: %w", err)
	}

	client := &acme.Client{
		Key:          accountKey,
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
	}

	tlsCfg := m.store.GetTLSConfig()
	email := strings.TrimSpace(tlsCfg.Email)
	if store.IsForbiddenEmail(email) {
		email = ""
	}
	if email == "" {
		envEmail := strings.TrimSpace(os.Getenv("EMAIL"))
		if !store.IsForbiddenEmail(envEmail) {
			email = envEmail
		}
	}
	var contacts []string
	if email != "" {
		contacts = []string{"mailto:" + email}
	}
	account := &acme.Account{Contact: contacts}
	_, _ = client.Register(ctx, account, acme.AcceptTOS)

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return err
	}

	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return err
		}
		if authz.Status == acme.StatusValid {
			continue
		}

		var httpChal *acme.Challenge
		for _, ch := range authz.Challenges {
			if ch.Type == "http-01" {
				httpChal = ch
				break
			}
		}
		if httpChal == nil {
			return fmt.Errorf("no http-01 challenge provided by ACME server for %s", domain)
		}

		resp, err := client.HTTP01ChallengeResponse(httpChal.Token)
		if err != nil {
			return fmt.Errorf("failed to compute challenge response: %w", err)
		}
		chalPath := client.HTTP01ChallengePath(httpChal.Token)

		m.mu.Lock()
		m.httpTokens[chalPath] = resp
		m.mu.Unlock()

		defer func(p string) {
			m.mu.Lock()
			delete(m.httpTokens, p)
			m.mu.Unlock()
		}(chalPath)

		if _, err := client.Accept(ctx, httpChal); err != nil {
			return fmt.Errorf("failed to accept http-01 challenge: %w", err)
		}

		authzRes, err := client.WaitAuthorization(ctx, authz.URI)
		if err != nil {
			return fmt.Errorf("http-01 challenge authorization failed: %w", err)
		}
		if authzRes.Status != acme.StatusValid {
			return fmt.Errorf("authorization not valid: status %s", authzRes.Status)
		}
	}

	order, err = client.WaitOrder(ctx, order.URI)
	if err != nil {
		return fmt.Errorf("failed to finalize ACME order: %w", err)
	}

	certPrivKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate EC key for certificate: %w", err)
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		DNSNames: []string{domain},
	}, certPrivKey)
	if err != nil {
		return fmt.Errorf("failed to generate CSR: %w", err)
	}

	chain, _, err := client.CreateCertFromOrder(ctx, order, csrBytes, true)
	if err != nil {
		return fmt.Errorf("failed to retrieve certificate chain from order: %w", err)
	}

	var buf bytes.Buffer
	keyDer, err := x509.MarshalECPrivateKey(certPrivKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	_ = pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})
	for _, certDer := range chain {
		_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: certDer})
	}

	certPath := filepath.Join(m.certDir, domain)
	if err := os.WriteFile(certPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to persist certificate to %s: %w", certPath, err)
	}

	return nil
}

func (m *Manager) getOrCreateACMEAccountKey() (crypto.Signer, error) {
	keyPath := filepath.Join(m.certDir, "acme_account+key")
	if data, err := os.ReadFile(keyPath); err == nil {
		if signer, err := loadPrivateKey(data); err == nil {
			return signer, nil
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	_ = os.WriteFile(keyPath, pemBytes, 0600)
	return key, nil
}

func loadPrivateKey(data []byte) (crypto.Signer, error) {
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			if signer, ok := key.(crypto.Signer); ok {
				return signer, nil
			}
		}
		data = rest
	}
	return nil, fmt.Errorf("no valid private key found")
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
	} else if strings.Contains(errStr, "certificate is not trusted") {
		reason = "System root certificates unavailable. Ensure system CA store is installed."
	} else if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "timeout") || strings.Contains(errStr, "no such host") {
		reason = "Let's Encrypt could not connect to your server from the internet. Verify router forwards port 80/443 and DNS points to public IP."
	} else if strings.Contains(errStr, "rate limited") || strings.Contains(errStr, "rateLimited") || strings.Contains(errStr, "too many failed authorizations") {
		reason = "Let's Encrypt rate limit reached (too many failed authorizations within 1 hour). Please wait for the cooldown period before requesting new certificates."
	} else if strings.Contains(errStr, "NXDOMAIN") {
		reason = "Domain does not exist in public DNS. Ensure Cloudflare DDNS has synced your public IP."
	} else if strings.Contains(errStr, "unauthorized") {
		reason = "ACME HTTP-01 challenge unauthorized. Ensure incoming traffic on port 80 reaches ProxyServer without being blocked or redirected."
	} else if strings.Contains(errStr, "no viable challenge type") {
		reason = "No viable ACME challenge type found. Ensure port 80 HTTP-01 challenge handler is reachable."
	}

	if reason != "" {
		return fmt.Sprintf("%v | Cause: %s", err, reason)
	}
	return errStr
}

// getOrCreateSelfSigned returns a self-signed fallback certificate for localhost
func (m *Manager) getOrCreateSelfSigned() (*tls.Certificate, error) {
	return m.getOrCreateSelfSignedForHost("localhost")
}

// getOrCreateSelfSignedForHost returns a cached or dynamically generated self-signed certificate matching host
func (m *Manager) getOrCreateSelfSignedForHost(host string) (*tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cleanHost := strings.ToLower(strings.TrimSpace(host))
	if cleanHost == "" {
		cleanHost = "localhost"
	}

	if m.selfSignedCerts == nil {
		m.selfSignedCerts = make(map[string]*tls.Certificate)
	}

	if cert, ok := m.selfSignedCerts[cleanHost]; ok {
		return cert, nil
	}

	// Generate in-memory self-signed certificate using ECDSA (instant generation, modern curve)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	dnsNames := []string{cleanHost}
	if cleanHost != "localhost" {
		dnsNames = append(dnsNames, "localhost", "*.localhost")
	} else {
		dnsNames = append(dnsNames, "*.localhost", "home.local")
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"ProxyServer Self-Signed Gateway"},
			CommonName:   cleanHost,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	m.selfSignedCerts[cleanHost] = &cert
	if cleanHost == "localhost" {
		m.selfSignedCert = &cert
	}
	return &cert, nil
}

// UpdateEmail updates the autocert ACME account email
func (m *Manager) UpdateEmail(email string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autocertMgr.Email = email
}

// DeleteCertificate removes all cached certificate files and in-memory caches for domain
func (m *Manager) DeleteCertificate(domain string) error {
	cleanHost := strings.ToLower(strings.TrimSpace(domain))
	if cleanHost == "" {
		return nil
	}

	m.mu.Lock()
	delete(m.selfSignedCerts, cleanHost)
	delete(m.lastErrors, cleanHost)
	delete(m.renewing, cleanHost)
	m.mu.Unlock()

	// Candidate certificate cache files on disk
	candidates := []string{
		filepath.Join(m.certDir, cleanHost),
		filepath.Join(m.certDir, cleanHost+"+rsa"),
		filepath.Join(m.certDir, cleanHost+"+key"),
		filepath.Join(m.certDir, cleanHost+"+token"),
	}

	for _, certPath := range candidates {
		if err := os.Remove(certPath); err == nil {
			logger.SystemInfo("TLS", "Deleted certificate cache file %s", certPath)
		}
	}

	logger.SystemInfo("TLS", "Purged SSL certificate and cache for domain '%s'", cleanHost)
	return nil
}

