package tlsmanager

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
