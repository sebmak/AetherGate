package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebmak/proxyserver/pkg/store"
)

func TestProxyEngineRoutingAndHeaders(t *testing.T) {
	// 1. Create a mock upstream HTTP server
	upstreamReceivedHeader := ""
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamReceivedHeader = r.Header.Get("X-Custom-Header")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"upstream":"success"}`))
	}))
	defer upstreamServer.Close()

	// 2. Setup mock Store
	tempDir, err := os.MkdirTemp("", "proxy-test-*")
	if err != nil {
		t.Fatalf("TempDir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	s, err := store.NewStore(filepath.Join(tempDir, "config.json"), "")
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	upstreamHost := upstreamServer.Listener.Addr().String()

	target := &store.ProxyTarget{
		Name:    "Test Upstream",
		Domain:  "test.makinson.me",
		Host:    upstreamHost,
		Scheme:  "http",
		Enabled: true,
		CustomHeaders: map[string]string{
			"X-Custom-Header": "ProxyServer-Value",
		},
		PassHostHeader: true,
	}
	if err := s.SaveTarget(target); err != nil {
		t.Fatalf("SaveTarget error: %v", err)
	}

	// 3. Create Proxy Engine
	hc := NewHealthChecker(s)
	engine := NewEngine(s, hc)

	// 4. Test matched domain request
	req := httptest.NewRequest("GET", "http://test.makinson.me/api/test", nil)
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from upstream, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if string(body) != `{"upstream":"success"}` {
		t.Fatalf("Unexpected upstream body: %s", string(body))
	}

	if upstreamReceivedHeader != "ProxyServer-Value" {
		t.Fatalf("Custom header was not forwarded: %s", upstreamReceivedHeader)
	}

	// 5. Test unconfigured domain request (should be 404)
	req404 := httptest.NewRequest("GET", "http://unknown.domain.com/", nil)
	rec404 := httptest.NewRecorder()

	engine.ServeHTTP(rec404, req404)
	if rec404.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 for unconfigured domain, got %d", rec404.Result().StatusCode)
	}

	// 6. Test metrics
	metrics := engine.GetMetrics().GetSnapshot()
	if metrics.TotalRequests < 2 {
		t.Fatalf("Expected at least 2 requests counted, got %d", metrics.TotalRequests)
	}
}

func TestHealthCheckerProbe(t *testing.T) {
	// Start an upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	}))
	defer upstreamServer.Close()

	tempDir, _ := os.MkdirTemp("", "hc-test-*")
	defer os.RemoveAll(tempDir)

	s, _ := store.NewStore(filepath.Join(tempDir, "config.json"), "")
	hc := NewHealthChecker(s)

	ok, latency, msg := hc.TestUpstream("http", upstreamServer.Listener.Addr().String())
	if !ok {
		t.Fatalf("TestUpstream should report healthy: %s", msg)
	}
	if latency < 0 {
		t.Fatalf("Latency should be non-negative: %d", latency)
	}
}
