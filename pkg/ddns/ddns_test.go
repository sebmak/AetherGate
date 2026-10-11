package ddns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebmak/proxyserver/pkg/store"
)

// TestDDNSDeleteDomainRecordWithoutCloudflare verifies that DeleteDomainRecord cleanly
// purges local in-memory records even when Cloudflare is disabled.
func TestDDNSDeleteDomainRecordWithoutCloudflare(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ddns-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	st, err := store.NewStore(configPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	mgr := NewManager(st)

	// Populate in-memory record
	mgr.mu.Lock()
	mgr.records["movies.home.makinson.me"] = &DNSRecordStatus{
		Domain:   "movies.home.makinson.me",
		RecordID: "rec123",
		Status:   "synced",
	}
	mgr.mu.Unlock()

	ctx := context.Background()
	if err := mgr.DeleteDomainRecord(ctx, "movies.home.makinson.me"); err != nil {
		t.Fatalf("DeleteDomainRecord failed: %v", err)
	}

	mgr.mu.RLock()
	if _, found := mgr.records["movies.home.makinson.me"]; found {
		t.Errorf("Expected in-memory record to be purged")
	}
	mgr.mu.RUnlock()
}

// TestDDNSDeleteDomainRecordWithMockAPI verifies that DeleteDomainRecord invokes the Cloudflare DELETE endpoint.
func TestDDNSDeleteDomainRecordWithMockAPI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ddns-test-api-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	deletedRecordID := ""

	// Mock Cloudflare API server
	// Mock Cloudflare API in-memory handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Zone lookup
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/client/v4/zones") && !strings.Contains(r.URL.Path, "dns_records") {
			resp := cfResponse{
				Success: true,
				Result: json.RawMessage(`[{
					"id": "zone123",
					"name": "makinson.me",
					"status": "active"
				}]`),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// 2. DNS records lookup
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/dns_records") {
			resp := cfResponse{
				Success: true,
				Result: json.RawMessage(`[{
					"id": "rec456",
					"type": "A",
					"name": "test.makinson.me",
					"content": "1.2.3.4"
				}]`),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// 3. DNS record deletion
		if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/dns_records/") {
			parts := strings.Split(r.URL.Path, "/")
			deletedRecordID = parts[len(parts)-1]
			resp := cfResponse{
				Success: true,
				Result:  json.RawMessage(`{"id": "` + deletedRecordID + `"}`),
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	})

	configPath := filepath.Join(tempDir, "config.json")
	st, err := store.NewStore(configPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	_ = st.SaveCloudflareConfig(store.CloudflareConfig{
		Enabled:  true,
		ApiToken: "mock-token",
		ZoneID:   "zone123",
		Domains:  []string{"test.makinson.me", "other.makinson.me"},
	})

	mgr := NewManager(st)
	// Pure in-memory transport avoids socket dialing in sandboxed test environments
	mgr.client = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Result(), nil
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.DeleteDomainRecord(ctx, "test.makinson.me"); err != nil {
		t.Fatalf("DeleteDomainRecord failed: %v", err)
	}

	if deletedRecordID != "rec456" {
		t.Errorf("Expected deleted record ID 'rec456', got '%s'", deletedRecordID)
	}

	// Verify domain was removed from config.Domains
	cfg := st.GetCloudflareConfig()
	for _, d := range cfg.Domains {
		if d == "test.makinson.me" {
			t.Errorf("Domain 'test.makinson.me' should have been removed from config.Domains")
		}
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
