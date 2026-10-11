package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/store"
)

// TargetHealth holds live reachability and latency metrics for an upstream
type TargetHealth struct {
	TargetID    string    `json:"target_id"`
	Domain      string    `json:"domain"`
	Upstream    string    `json:"upstream"`
	IsHealthy   bool      `json:"is_healthy"`
	LatencyMs   int64     `json:"latency_ms"`
	LastChecked time.Time `json:"last_checked"`
	LastError   string    `json:"last_error,omitempty"`
}

// HealthChecker monitors upstream targets periodically
type HealthChecker struct {
	mu       sync.RWMutex
	store    *store.Store
	client   *http.Client
	health   map[string]*TargetHealth
	stopChan chan struct{}
}

// NewHealthChecker creates a HealthChecker
func NewHealthChecker(s *store.Store) *HealthChecker {
	return &HealthChecker{
		store: s,
		client: &http.Client{
			Timeout: 4 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Don't follow redirects during health check
				return http.ErrUseLastResponse
			},
		},
		health: make(map[string]*TargetHealth),
	}
}

// Start begins background health check probes
func (hc *HealthChecker) Start(ctx context.Context) {
	hc.mu.Lock()
	hc.stopChan = make(chan struct{})
	hc.mu.Unlock()

	// Initial probe
	go hc.probeAll()

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-hc.stopChan:
				return
			case <-ticker.C:
				hc.probeAll()
			}
		}
	}()
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if hc.stopChan != nil {
		close(hc.stopChan)
		hc.stopChan = nil
	}
}

func (hc *HealthChecker) probeAll() {
	targets := hc.store.GetTargets()
	var wg sync.WaitGroup

	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		wg.Add(1)
		go func(target *store.ProxyTarget) {
			defer wg.Done()
			h := hc.checkTarget(target)
			hc.mu.Lock()
			hc.health[target.ID] = h
			hc.mu.Unlock()
		}(t)
	}

	wg.Wait()
}

func (hc *HealthChecker) checkTarget(target *store.ProxyTarget) *TargetHealth {
	start := time.Now()
	targetURL := fmt.Sprintf("%s://%s", target.Scheme, target.Host)

	// First attempt HTTP request
	req, err := http.NewRequestWithContext(context.Background(), "GET", targetURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "ProxyServer-HealthCheck/1.0")
		resp, err := hc.client.Do(req)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			_ = resp.Body.Close()
			return &TargetHealth{
				TargetID:    target.ID,
				Domain:      target.Domain,
				Upstream:    targetURL,
				IsHealthy:   true,
				LatencyMs:   latency,
				LastChecked: time.Now(),
			}
		}
	}

	// Fallback to TCP dial if HTTP had an error (e.g. self-signed TLS or non-HTTP endpoint)
	hostPort := target.Host
	if !strings.Contains(hostPort, ":") {
		if target.Scheme == "https" {
			hostPort += ":443"
		} else {
			hostPort += ":80"
		}
	}

	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, dialErr := dialer.Dial("tcp", hostPort)
	latency := time.Since(start).Milliseconds()

	if dialErr == nil {
		_ = conn.Close()
		return &TargetHealth{
			TargetID:    target.ID,
			Domain:      target.Domain,
			Upstream:    targetURL,
			IsHealthy:   true,
			LatencyMs:   latency,
			LastChecked: time.Now(),
		}
	}

	return &TargetHealth{
		TargetID:    target.ID,
		Domain:      target.Domain,
		Upstream:    targetURL,
		IsHealthy:   false,
		LatencyMs:   latency,
		LastChecked: time.Now(),
		LastError:   dialErr.Error(),
	}
}

// GetHealth returns map of all current target health statuses
func (hc *HealthChecker) GetHealth() map[string]*TargetHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	result := make(map[string]*TargetHealth, len(hc.health))
	for k, v := range hc.health {
		c := *v
		result[k] = &c
	}
	return result
}

// TestUpstream directly probes a given scheme and host immediately
func (hc *HealthChecker) TestUpstream(scheme, host string) (bool, int64, string) {
	if scheme == "" {
		scheme = "http"
	}
	targetURL := fmt.Sprintf("%s://%s", scheme, host)
	start := time.Now()

	req, err := http.NewRequestWithContext(context.Background(), "GET", targetURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "ProxyServer-HealthCheck/1.0")
		resp, err := hc.client.Do(req)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			_ = resp.Body.Close()
			return true, latency, fmt.Sprintf("HTTP %d OK", resp.StatusCode)
		}
	}

	// Fallback TCP probe
	u, err := url.Parse(targetURL)
	hostPort := host
	if err == nil && u.Host != "" {
		hostPort = u.Host
	}
	if !strings.Contains(hostPort, ":") {
		if scheme == "https" {
			hostPort += ":443"
		} else {
			hostPort += ":80"
		}
	}

	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, dialErr := dialer.Dial("tcp", hostPort)
	latency := time.Since(start).Milliseconds()

	if dialErr == nil {
		_ = conn.Close()
		return true, latency, "TCP connection successful"
	}

	return false, latency, dialErr.Error()
}
