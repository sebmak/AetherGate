package ddns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/store"
)

// DNSRecordStatus represents the current status of a managed DNS record
type DNSRecordStatus struct {
	Domain      string    `json:"domain"`
	RecordID    string    `json:"record_id"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	Proxied     bool      `json:"proxied"`
	Status      string    `json:"status"` // "synced", "updated", "error", "pending"
	Message     string    `json:"message,omitempty"`
	LastUpdated time.Time `json:"last_updated"`
}

// SyncLogEntry represents an event in the DDNS sync history
type SyncLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	Updated   []string  `json:"updated,omitempty"`
}

// Status represents the current overall state of the DDNS engine
type Status struct {
	Enabled       bool                        `json:"enabled"`
	CurrentIP     string                      `json:"current_ip"`
	PreviousIP    string                      `json:"previous_ip"`
	LastCheckTime time.Time                   `json:"last_check_time"`
	LastSyncTime  time.Time                   `json:"last_sync_time"`
	LastError     string                      `json:"last_error,omitempty"`
	Records       map[string]*DNSRecordStatus `json:"records"`
	History       []*SyncLogEntry             `json:"history"`
}

// Manager coordinates Cloudflare DDNS checking and synchronization
type Manager struct {
	mu            sync.RWMutex
	store         *store.Store
	client        *http.Client
	currentIP     string
	previousIP    string
	lastCheckTime time.Time
	lastSyncTime  time.Time
	lastError     string
	records       map[string]*DNSRecordStatus
	history       []*SyncLogEntry
	stopChan      chan struct{}
}

// NewManager creates a DDNS Manager
func NewManager(s *store.Store) *Manager {
	return &Manager{
		store: s,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		records: make(map[string]*DNSRecordStatus),
		history: make([]*SyncLogEntry, 0, 50),
	}
}

// Start begins background synchronization
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.stopChan = make(chan struct{})
	m.mu.Unlock()

	// Initial check on boot
	go func() {
		time.Sleep(3 * time.Second)
		_ = m.Sync(context.Background())
	}()

	go func() {
		for {
			cfg := m.store.GetCloudflareConfig()
			interval := cfg.IntervalSeconds
			if interval < 60 {
				interval = 300 // default 5m
			}

			select {
			case <-ctx.Done():
				return
			case <-m.stopChan:
				return
			case <-time.After(time.Duration(interval) * time.Second):
				if cfg.Enabled {
					_ = m.Sync(context.Background())
				}
			}
		}
	}()
}

// Stop terminates background synchronization
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopChan != nil {
		close(m.stopChan)
		m.stopChan = nil
	}
}

// TriggerSyncNow initiates an immediate background sync
func (m *Manager) TriggerSyncNow() {
	go func() {
		_ = m.Sync(context.Background())
	}()
}

// EnsureDomain ensures a single domain is synced to current IP immediately
func (m *Manager) EnsureDomain(domain string) error {
	cfg := m.store.GetCloudflareConfig()
	if !cfg.Enabled || (cfg.ApiToken == "" && cfg.ApiKey == "") {
		return nil
	}

	ip, err := m.GetPublicIP(context.Background())
	if err != nil {
		return err
	}

	return m.syncDomain(context.Background(), cfg, domain, ip)
}

// GetStatus returns the current DDNS manager state
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.GetCloudflareConfig()

	recordsCopy := make(map[string]*DNSRecordStatus, len(m.records))
	for k, v := range m.records {
		r := *v
		recordsCopy[k] = &r
	}

	historyCopy := make([]*SyncLogEntry, len(m.history))
	for i, h := range m.history {
		e := *h
		historyCopy[i] = &e
	}

	return Status{
		Enabled:       cfg.Enabled,
		CurrentIP:     m.currentIP,
		PreviousIP:    m.previousIP,
		LastCheckTime: m.lastCheckTime,
		LastSyncTime:  m.lastSyncTime,
		LastError:     m.lastError,
		Records:       recordsCopy,
		History:       historyCopy,
	}
}

// GetPublicIP retrieves the server's current external public IPv4 address
func (m *Manager) GetPublicIP(ctx context.Context) (string, error) {
	endpoints := []string{
		"https://api.ipify.org?format=text",
		"https://1.1.1.1/cdn-cgi/trace",
		"https://icanhazip.com",
		"https://checkip.amazonaws.com",
	}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "ProxyServer-DDNS/1.0")

		resp, err := m.client.Do(req)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			continue
		}

		raw := string(body)
		if strings.Contains(endpoint, "cdn-cgi/trace") {
			// Extract ip=1.2.3.4
			lines := strings.Split(raw, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "ip=") {
					ipCandidate := strings.TrimPrefix(line, "ip=")
					if parsed := net.ParseIP(strings.TrimSpace(ipCandidate)); parsed != nil {
						return parsed.String(), nil
					}
				}
			}
		} else {
			ipCandidate := strings.TrimSpace(raw)
			if parsed := net.ParseIP(ipCandidate); parsed != nil {
				return parsed.String(), nil
			}
		}
	}

	return "", errors.New("failed to determine public IP from all external check providers")
}

// Sync performs a full DDNS synchronization round
func (m *Manager) Sync(ctx context.Context) error {
	m.mu.Lock()
	m.lastCheckTime = time.Now()
	m.mu.Unlock()

	cfg := m.store.GetCloudflareConfig()
	if !cfg.Enabled {
		return nil
	}

	if cfg.ApiToken == "" && (cfg.ApiKey == "" || cfg.Email == "") {
		err := errors.New("cloudflare API credentials not configured")
		m.recordError(err.Error())
		return err
	}

	ip, err := m.GetPublicIP(ctx)
	if err != nil {
		m.recordError(fmt.Sprintf("Failed to fetch public IP: %v", err))
		return err
	}

	m.mu.Lock()
	if m.currentIP != ip {
		m.previousIP = m.currentIP
		m.currentIP = ip
	}
	m.lastError = ""
	m.mu.Unlock()

	// Gather domains to sync
	domainsMap := make(map[string]struct{})
	for _, d := range cfg.Domains {
		clean := strings.ToLower(strings.TrimSpace(d))
		if clean != "" {
			domainsMap[clean] = struct{}{}
		}
	}

	if cfg.AutoSyncProxies {
		for _, target := range m.store.GetTargets() {
			if target.Enabled && target.CloudflareSync {
				clean := strings.ToLower(strings.TrimSpace(target.Domain))
				if clean != "" && !strings.HasPrefix(clean, "*.") {
					domainsMap[clean] = struct{}{}
				}
			}
		}
	}

	if len(domainsMap) == 0 {
		m.addHistory(ip, true, "No domains configured for DDNS sync", nil)
		return nil
	}

	var updatedDomains []string
	var syncErrors []string

	for domain := range domainsMap {
		if err := m.syncDomain(ctx, cfg, domain, ip); err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("%s: %v", domain, err))
		} else {
			updatedDomains = append(updatedDomains, domain)
		}
	}

	m.mu.Lock()
	m.lastSyncTime = time.Now()
	m.mu.Unlock()

	if len(syncErrors) > 0 {
		msg := fmt.Sprintf("Synced %d domains with errors: %s", len(updatedDomains), strings.Join(syncErrors, "; "))
		m.recordError(msg)
		m.addHistory(ip, false, msg, updatedDomains)
		return errors.New(msg)
	}

	msg := fmt.Sprintf("Successfully verified %d DNS record(s) pointing to %s", len(updatedDomains), ip)
	m.addHistory(ip, true, msg, updatedDomains)
	logger.Info("[DDNS] %s", msg)
	return nil
}

func (m *Manager) syncDomain(ctx context.Context, cfg store.CloudflareConfig, domain, ip string) error {
	zoneID := cfg.ZoneID
	if zoneID == "" {
		// Auto-discover zone ID if not explicitly specified
		resolvedZoneID, err := m.ResolveZoneID(ctx, cfg, domain)
		if err != nil {
			m.setRecordStatus(domain, "", "A", ip, cfg.Proxied, "error", err.Error())
			return err
		}
		zoneID = resolvedZoneID
	}

	// 1. Fetch existing DNS records for this domain
	records, err := m.getDNSRecords(ctx, cfg, zoneID, domain, "A")
	if err != nil {
		m.setRecordStatus(domain, "", "A", ip, cfg.Proxied, "error", err.Error())
		return err
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 1 // 1 = Auto in Cloudflare
	}

	if len(records) == 0 {
		// Create record
		recID, err := m.createDNSRecord(ctx, cfg, zoneID, domain, "A", ip, cfg.Proxied, ttl)
		if err != nil {
			m.setRecordStatus(domain, "", "A", ip, cfg.Proxied, "error", err.Error())
			return err
		}
		m.setRecordStatus(domain, recID, "A", ip, cfg.Proxied, "updated", "Created new A record")
		logger.Info("[DDNS] Created DNS record for %s -> %s", domain, ip)
		return nil
	}

	// Check if update is needed
	rec := records[0]
	if rec.Content == ip && rec.Proxied == cfg.Proxied {
		m.setRecordStatus(domain, rec.ID, "A", ip, cfg.Proxied, "synced", "Record is up-to-date")
		return nil
	}

	// Update existing record
	err = m.updateDNSRecord(ctx, cfg, zoneID, rec.ID, domain, "A", ip, cfg.Proxied, ttl)
	if err != nil {
		m.setRecordStatus(domain, rec.ID, "A", ip, cfg.Proxied, "error", err.Error())
		return err
	}

	m.setRecordStatus(domain, rec.ID, "A", ip, cfg.Proxied, "updated", fmt.Sprintf("Updated IP %s -> %s", rec.Content, ip))
	logger.Info("[DDNS] Updated DNS record for %s (%s -> %s)", domain, rec.Content, ip)
	return nil
}

func (m *Manager) setRecordStatus(domain, recordID, recType, content string, proxied bool, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records[domain] = &DNSRecordStatus{
		Domain:      domain,
		RecordID:    recordID,
		Type:        recType,
		Content:     content,
		Proxied:     proxied,
		Status:      status,
		Message:     message,
		LastUpdated: time.Now(),
	}
}

func (m *Manager) recordError(err string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = err
}

func (m *Manager) addHistory(ip string, success bool, message string, updated []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := &SyncLogEntry{
		Timestamp: time.Now(),
		IP:        ip,
		Success:   success,
		Message:   message,
		Updated:   updated,
	}

	if len(m.history) >= 50 {
		m.history = m.history[1:]
	}
	m.history = append(m.history, entry)
}

// Cloudflare API Request/Response Types
type cfResponse struct {
	Success  bool            `json:"success"`
	Errors   []cfError       `json:"errors"`
	Messages []string        `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

func (m *Manager) cfRequest(ctx context.Context, cfg store.CloudflareConfig, method, path string, body any) (*cfResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	url := "https://api.cloudflare.com/client/v4" + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if cfg.ApiToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.ApiToken)
	} else if cfg.ApiKey != "" && cfg.Email != "" {
		req.Header.Set("X-Auth-Key", cfg.ApiKey)
		req.Header.Set("X-Auth-Email", cfg.Email)
	} else {
		return nil, errors.New("no Cloudflare credentials provided")
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cfResp cfResponse
	if err := json.Unmarshal(respBytes, &cfResp); err != nil {
		return nil, fmt.Errorf("invalid json response from Cloudflare (status %d): %w", resp.StatusCode, err)
	}

	if !cfResp.Success {
		var errMsgs []string
		for _, e := range cfResp.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
		}
		return nil, fmt.Errorf("cloudflare API error: %s", strings.Join(errMsgs, ", "))
	}

	return &cfResp, nil
}

// VerifyToken validates the Cloudflare API token
func (m *Manager) VerifyToken(ctx context.Context, token string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.cloudflare.com/client/v4/user/tokens/verify", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := m.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var cfResp cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfResp); err != nil {
		return false, err
	}

	return cfResp.Success, nil
}

// ListZones returns active zones for the account
func (m *Manager) ListZones(ctx context.Context, cfg store.CloudflareConfig) ([]cfZone, error) {
	resp, err := m.cfRequest(ctx, cfg, "GET", "/zones?status=active&per_page=50", nil)
	if err != nil {
		return nil, err
	}

	var zones []cfZone
	if err := json.Unmarshal(resp.Result, &zones); err != nil {
		return nil, err
	}

	return zones, nil
}

// ResolveZoneID looks up the best matching ZoneID for a given domain
func (m *Manager) ResolveZoneID(ctx context.Context, cfg store.CloudflareConfig, domain string) (string, error) {
	zones, err := m.ListZones(ctx, cfg)
	if err != nil {
		return "", err
	}

	domainClean := strings.ToLower(strings.TrimSpace(domain))
	var bestMatch string
	var bestMatchLen int

	for _, z := range zones {
		zName := strings.ToLower(z.Name)
		if domainClean == zName || strings.HasSuffix(domainClean, "."+zName) {
			if len(zName) > bestMatchLen {
				bestMatch = z.ID
				bestMatchLen = len(zName)
			}
		}
	}

	if bestMatch != "" {
		return bestMatch, nil
	}

	return "", fmt.Errorf("no matching Cloudflare zone found for domain %s", domain)
}

func (m *Manager) getDNSRecords(ctx context.Context, cfg store.CloudflareConfig, zoneID, name, recordType string) ([]cfDNSRecord, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&type=%s", zoneID, name, recordType)
	resp, err := m.cfRequest(ctx, cfg, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var records []cfDNSRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return nil, err
	}

	return records, nil
}

func (m *Manager) createDNSRecord(ctx context.Context, cfg store.CloudflareConfig, zoneID, name, recordType, content string, proxied bool, ttl int) (string, error) {
	path := fmt.Sprintf("/zones/%s/dns_records", zoneID)
	payload := map[string]any{
		"type":    recordType,
		"name":    name,
		"content": content,
		"proxied": proxied,
		"ttl":     ttl,
	}

	resp, err := m.cfRequest(ctx, cfg, "POST", path, payload)
	if err != nil {
		return "", err
	}

	var rec cfDNSRecord
	if err := json.Unmarshal(resp.Result, &rec); err != nil {
		return "", err
	}

	return rec.ID, nil
}

func (m *Manager) updateDNSRecord(ctx context.Context, cfg store.CloudflareConfig, zoneID, recordID, name, recordType, content string, proxied bool, ttl int) error {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID)
	payload := map[string]any{
		"type":    recordType,
		"name":    name,
		"content": content,
		"proxied": proxied,
		"ttl":     ttl,
	}

	_, err := m.cfRequest(ctx, cfg, "PUT", path, payload)
	return err
}
