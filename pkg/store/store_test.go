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
	// Initialize Store without auto-importing any files
	s, err := NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if len(s.GetTargets()) != 0 {
		t.Fatalf("Expected 0 targets initially, got %d", len(s.GetTargets()))
	}

	// Import legacy servers.json payload explicitly
	imported, err := s.ImportLegacyPayload([]byte(mockLegacy))
	if err != nil {
		t.Fatalf("ImportLegacyPayload failed: %v", err)
	}
	if imported != 2 {
		t.Fatalf("Expected 2 imported targets, got %d", imported)
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

// TestStoreTargetsSortedByName verifies that GetTargets always returns targets alphabetically ordered by name
func TestStoreTargetsSortedByName(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "proxyserver-sort-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	s, err := NewStore(configPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// Insert in non-alphabetical order
	names := []string{"Zebra", "Alpha", "Beta", "Gamma"}
	for _, name := range names {
		if err := s.SaveTarget(&ProxyTarget{
			Name:    name,
			Domain:  name + ".example.com",
			Host:    "127.0.0.1:8080",
			Enabled: true,
		}); err != nil {
			t.Fatalf("Failed to save target %s: %v", name, err)
		}
	}

	targets := s.GetTargets()
	if len(targets) != 4 {
		t.Fatalf("Expected 4 targets, got %d", len(targets))
	}

	expected := []string{"Alpha", "Beta", "Gamma", "Zebra"}
	for i, exp := range expected {
		if targets[i].Name != exp {
			t.Errorf("At index %d: expected name '%s', got '%s'", i, exp, targets[i].Name)
		}
	}
}

