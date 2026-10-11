package tlsmanager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sebmak/proxyserver/pkg/store"
)

// TestTLSManagerHostPolicy verifies that the HostPolicy correctly handles domains with and without ports,
// case insensitivity, and registered vs unregistered hosts.
func TestTLSManagerHostPolicy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	certDir := filepath.Join(tmpDir, "certs")

	st, err := store.NewStore(cfgPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Add test target
	err = st.SaveTarget(&store.ProxyTarget{
		Domain:  "movies.home.makinson.me",
		Host:    "192.168.1.104:9990",
		Scheme:  "http",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to save target: %v", err)
	}

	mgr, err := NewManager(st, certDir)
	if err != nil {
		t.Fatalf("Failed to create TLS manager: %v", err)
	}

	ctx := context.Background()

	// 1. Direct domain match
	if err := mgr.autocertMgr.HostPolicy(ctx, "movies.home.makinson.me"); err != nil {
		t.Errorf("Expected domain to match, got: %v", err)
	}

	// 2. Case insensitive match
	if err := mgr.autocertMgr.HostPolicy(ctx, "MOVIES.HOME.MAKINSON.ME"); err != nil {
		t.Errorf("Expected uppercase domain to match, got: %v", err)
	}

	// 3. Match with port 80
	if err := mgr.autocertMgr.HostPolicy(ctx, "movies.home.makinson.me:80"); err != nil {
		t.Errorf("Expected domain with port 80 to match, got: %v", err)
	}

	// 4. Match with port 9080
	if err := mgr.autocertMgr.HostPolicy(ctx, "movies.home.makinson.me:9080"); err != nil {
		t.Errorf("Expected domain with port 9080 to match, got: %v", err)
	}

	// 5. Unregistered domain should fail
	if err := mgr.autocertMgr.HostPolicy(ctx, "unregistered.example.com"); err == nil {
		t.Errorf("Expected unregistered domain to be rejected, but it succeeded")
	}
}

// TestTLSManagerCertificateInfoCacheLookup verifies that GetCertificateInfo correctly discovers
// both standard and +rsa cached certificates.
func TestTLSManagerCertificateInfoCacheLookup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tls-test-cache-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	certDir := filepath.Join(tmpDir, "certs")

	st, err := store.NewStore(cfgPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	err = st.SaveTarget(&store.ProxyTarget{
		Domain:  "test.example.com",
		Host:    "127.0.0.1:8080",
		Scheme:  "http",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to save target: %v", err)
	}

	mgr, err := NewManager(st, certDir)
	if err != nil {
		t.Fatalf("Failed to create TLS manager: %v", err)
	}

	// 1. Initial status should be self-signed dev certificate
	info := mgr.GetCertificateInfo("test.example.com")
	if info.Status != "self_signed" {
		t.Errorf("Expected status 'self_signed', got: %s", info.Status)
	}
	if !info.IsSelfSigned {
		t.Errorf("Expected IsSelfSigned to be true")
	}

	// 2. Generate a mock RSA certificate file at test.example.com+rsa signed by a mock Let's Encrypt CA
	caPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "R3",
			Organization: []string{"Let's Encrypt"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(12345),
		Subject: pkix.Name{
			CommonName: "test.example.com",
		},
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(60 * 24 * time.Hour),
		DNSNames:  []string{"test.example.com"},
	}
	derBytes, _ := x509.CreateCertificate(rand.Reader, &template, &caTemplate, &priv.PublicKey, caPriv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	rsaCertPath := filepath.Join(certDir, "test.example.com+rsa")
	if err := os.WriteFile(rsaCertPath, certPEM, 0600); err != nil {
		t.Fatalf("Failed to write RSA cert: %v", err)
	}

	// 3. Lookup should now find the +rsa certificate
	info = mgr.GetCertificateInfo("test.example.com")
	if info.Status != "valid" {
		t.Errorf("Expected status 'valid', got: %s", info.Status)
	}
	if info.IsSelfSigned {
		t.Errorf("Expected IsSelfSigned to be false for cached RSA certificate")
	}
	if info.Issuer != "Let's Encrypt (R3)" {
		t.Errorf("Expected Issuer 'Let's Encrypt (R3)', got: %s", info.Issuer)
	}
	if info.DaysRemaining < 58 || info.DaysRemaining > 61 {
		t.Errorf("Expected ~60 days remaining, got: %d", info.DaysRemaining)
	}
}

// TestTLSManagerGetCertificateDynamicHost verifies that GetCertificate dynamically serves
// a matching SAN and CN for incoming hostnames without freezing the handshake.
func TestTLSManagerGetCertificateDynamicHost(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tls-test-sni-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	certDir := filepath.Join(tmpDir, "certs")

	st, err := store.NewStore(cfgPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mgr, err := NewManager(st, certDir)
	if err != nil {
		t.Fatalf("Failed to create TLS manager: %v", err)
	}

	// 1. Verify TLS Config has modern curves
	cfg := mgr.GetTLSConfig()
	if cfg == nil || len(cfg.CurvePreferences) == 0 {
		t.Fatalf("Expected non-empty CurvePreferences in TLS config")
	}
	if cfg.CurvePreferences[0] != tls.X25519 {
		t.Errorf("Expected X25519 as preferred curve, got %v", cfg.CurvePreferences[0])
	}

	// 2. Request certificate for domain with no cached cert
	cert, err := mgr.GetCertificate(&tls.ClientHelloInfo{
		ServerName: "movies.home.makinson.me",
	})
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatalf("Returned certificate has no certificate data")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("Failed to parse returned certificate: %v", err)
	}

	if leaf.Subject.CommonName != "movies.home.makinson.me" {
		t.Errorf("Expected CommonName 'movies.home.makinson.me', got '%s'", leaf.Subject.CommonName)
	}

	hasHostSAN := false
	for _, san := range leaf.DNSNames {
		if san == "movies.home.makinson.me" {
			hasHostSAN = true
			break
		}
	}
	if !hasHostSAN {
		t.Errorf("Expected 'movies.home.makinson.me' in DNSNames, got %v", leaf.DNSNames)
	}

	// 3. Request again should return cached certificate
	cert2, err := mgr.GetCertificate(&tls.ClientHelloInfo{
		ServerName: "movies.home.makinson.me",
	})
	if err != nil {
		t.Fatalf("Second GetCertificate failed: %v", err)
	}
	if cert != cert2 {
		t.Errorf("Expected cached certificate pointer to be returned")
	}
}

// TestTLSManagerDeleteCertificate verifies that DeleteCertificate removes certificate files from disk
// and purges in-memory caches.
func TestTLSManagerDeleteCertificate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tls-test-del-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	certDir := filepath.Join(tmpDir, "certs")

	st, err := store.NewStore(cfgPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mgr, err := NewManager(st, certDir)
	if err != nil {
		t.Fatalf("Failed to create TLS manager: %v", err)
	}

	domain := "deleteme.home.makinson.me"

	// 1. Create mock certificate files on disk
	certFile := filepath.Join(certDir, domain)
	rsaFile := filepath.Join(certDir, domain+"+rsa")
	_ = os.WriteFile(certFile, []byte("mock-cert"), 0600)
	_ = os.WriteFile(rsaFile, []byte("mock-rsa"), 0600)

	// 2. Put in-memory entries
	mgr.mu.Lock()
	mgr.selfSignedCerts[domain] = &tls.Certificate{}
	mgr.lastErrors[domain] = "some error"
	mgr.renewing[domain] = true
	mgr.mu.Unlock()

	// 3. Delete certificate
	if err := mgr.DeleteCertificate(domain); err != nil {
		t.Fatalf("DeleteCertificate failed: %v", err)
	}

	// 4. Verify disk files are gone
	if _, err := os.Stat(certFile); !os.IsNotExist(err) {
		t.Errorf("Expected %s to be deleted", certFile)
	}
	if _, err := os.Stat(rsaFile); !os.IsNotExist(err) {
		t.Errorf("Expected %s to be deleted", rsaFile)
	}

	// 5. Verify in-memory state is cleared
	mgr.mu.RLock()
	if _, ok := mgr.selfSignedCerts[domain]; ok {
		t.Errorf("Expected selfSignedCerts[%s] to be deleted", domain)
	}
	if _, ok := mgr.lastErrors[domain]; ok {
		t.Errorf("Expected lastErrors[%s] to be deleted", domain)
	}
	if _, ok := mgr.renewing[domain]; ok {
		t.Errorf("Expected renewing[%s] to be deleted", domain)
	}
	mgr.mu.RUnlock()
}

