package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreImportLegacyAndCRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "proxyserver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	legacyPath := filepath.Join(tempDir, "servers.json")

	// Create mock legacy servers.json
	mockLegacy := `{
		"targets": {
			"movies.home.makinson.me": {
				"host": "192.168.1.104:9990",
				"scheme": "http",
				"force_ssl": true
			},
			"local.home.makinson.me": {
				"host": "127.0.0.1:8081",
				"scheme": "http",
				"force_ssl": false,
				"require_auth": true,
				"auth_resource": "server.local"
			}
		}
	}`
	if err := os.WriteFile(legacyPath, []byte(mockLegacy), 0644); err != nil {
		t.Fatalf("Failed to write mock legacy file: %v", err)
	}

	// Initialize Store with legacy import
	s, err := NewStore(configPath, legacyPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	targets := s.GetTargets()
	if len(targets) != 2 {
		t.Fatalf("Expected 2 imported targets, got %d", len(targets))
	}

	// Lookup by domain
	movies, found := s.GetTargetByDomain("movies.home.makinson.me")
	if !found || movies == nil {
		t.Fatalf("Target movies.home.makinson.me not found")
	}
	if movies.Host != "192.168.1.104:9990" || !movies.ForceSSL {
		t.Fatalf("Incorrect movies target settings: %+v", movies)
	}

	local, found := s.GetTargetByDomain("local.home.makinson.me")
	if !found || local == nil {
		t.Fatalf("Target local.home.makinson.me not found")
	}
	if !local.RequireAuth || local.AuthResource != "server.local" {
		t.Fatalf("Incorrect local target settings: %+v", local)
	}

	// Add new target dynamically
	newTarget := &ProxyTarget{
		Name:    "Nextcloud",
		Domain:  "cloud.home.makinson.me",
		Host:    "192.168.1.150:8443",
		Scheme:  "https",
		Enabled: true,
	}
	if err := s.SaveTarget(newTarget); err != nil {
		t.Fatalf("Failed to save new target: %v", err)
	}

	if len(s.GetTargets()) != 3 {
		t.Fatalf("Expected 3 targets after save, got %d", len(s.GetTargets()))
	}

	// Toggle target
	toggled, err := s.ToggleTarget(newTarget.ID)
	if err != nil || toggled.Enabled != false {
		t.Fatalf("Expected target to be disabled after toggle")
	}

	// Disabled target should not be found by GetTargetByDomain
	_, found = s.GetTargetByDomain("cloud.home.makinson.me")
	if found {
		t.Fatalf("Disabled target should not match GetTargetByDomain")
	}

	// Delete target
	if err := s.DeleteTarget(newTarget.ID); err != nil {
		t.Fatalf("Failed to delete target: %v", err)
	}

	if len(s.GetTargets()) != 2 {
		t.Fatalf("Expected 2 targets after delete, got %d", len(s.GetTargets()))
	}
}
