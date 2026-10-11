package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sebmak/proxyserver/pkg/logger"
)

// ProxyTarget defines an upstream target for reverse proxying
type ProxyTarget struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Domain          string            `json:"domain"`
	Host            string            `json:"host"`
	Scheme          string            `json:"scheme"`
	ForceSSL        bool              `json:"force_ssl"`
	RequireAuth     bool              `json:"require_auth"`
	AuthResource    string            `json:"auth_resource,omitempty"`
	CustomHeaders   map[string]string `json:"custom_headers,omitempty"`
	PassHostHeader  bool              `json:"pass_host_header"`
	Enabled         bool              `json:"enabled"`
	CloudflareSync  bool              `json:"cloudflare_sync"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// CloudflareConfig holds DDNS settings
type CloudflareConfig struct {
	Enabled          bool     `json:"enabled"`
	ApiToken         string   `json:"api_token"`
	ApiKey           string   `json:"api_key"`
	Email            string   `json:"email"`
	ZoneID           string   `json:"zone_id"`
	ZoneName         string   `json:"zone_name"`
	Proxied          bool     `json:"proxied"`
	TTL              int      `json:"ttl"`
	IntervalSeconds  int      `json:"interval_seconds"`
	Domains          []string `json:"domains"`
	AutoSyncProxies  bool     `json:"auto_sync_proxies"`
}

// SecurityConfig holds authentication and admin settings
type SecurityConfig struct {
	AdminUsername         string `json:"admin_username"`
	PasswordHash          string `json:"password_hash"`
	JwtSecret             string `json:"jwt_secret"`
	SessionDurationHours  int    `json:"session_duration_hours"`
	IsConfigured          bool   `json:"is_configured"`
}

// TLSConfig holds SSL/TLS settings
type TLSConfig struct {
	Mode     string `json:"mode"` // "autocert", "custom", "none"
	Email    string `json:"email"`
	CertPath string `json:"cert_path,omitempty"`
	KeyPath  string `json:"key_path,omitempty"`
}

// Config is the root persisted document
type Config struct {
	Version    int                     `json:"version"`
	Security   SecurityConfig          `json:"security"`
	TLS        TLSConfig               `json:"tls"`
	Cloudflare CloudflareConfig        `json:"cloudflare"`
	Targets    map[string]*ProxyTarget `json:"targets"` // Keyed by ID
}

// Store handles thread-safe configuration management and persistence
type Store struct {
	mu          sync.RWMutex
	configPath  string
	data        *Config
	changeHooks []func()
}

// IsForbiddenEmail checks if an email belongs to RFC 2606 or placeholder domains banned by ACME/Let's Encrypt
func IsForbiddenEmail(email string) bool {
	if email == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(email))
	forbiddenDomains := []string{
		"@example.com", "@example.org", "@example.net",
		"@test.com", "@test.org", "@localhost", "@invalid",
	}
	for _, fd := range forbiddenDomains {
		if strings.HasSuffix(lower, fd) {
			return true
		}
	}
	return false
}

// NewStore creates a new Store instance and loads existing data
func NewStore(configPath string) (*Store, error) {
	s := &Store{
		configPath: configPath,
		data: &Config{
			Version: 1,
			Security: SecurityConfig{
				SessionDurationHours: 72,
				JwtSecret:            generateRandomHex(32),
				IsConfigured:         false,
			},
			TLS: TLSConfig{
				Mode:  "autocert",
				Email: "",
			},
			Cloudflare: CloudflareConfig{
				Enabled:         false,
				Proxied:         true,
				TTL:             1,
				IntervalSeconds: 300,
				AutoSyncProxies: true,
				Domains:         []string{},
			},
			Targets: make(map[string]*ProxyTarget),
		},
		changeHooks: make([]func(), 0),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

// OnChange registers a hook called whenever targets or configuration change
func (s *Store) OnChange(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeHooks = append(s.changeHooks, hook)
}

func (s *Store) notifyChange() {
	for _, hook := range s.changeHooks {
		go hook()
	}
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	// Check if config.json exists
	if _, err := os.Stat(s.configPath); err == nil {
		content, err := os.ReadFile(s.configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		var cfg Config
		if err := json.Unmarshal(content, &cfg); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}

		if cfg.Targets == nil {
			cfg.Targets = make(map[string]*ProxyTarget)
		}
		if cfg.Security.JwtSecret == "" {
			cfg.Security.JwtSecret = generateRandomHex(32)
		}
		if cfg.Security.SessionDurationHours == 0 {
			cfg.Security.SessionDurationHours = 72
		}
		if cfg.Cloudflare.IntervalSeconds == 0 {
			cfg.Cloudflare.IntervalSeconds = 300
		}
		if cfg.Cloudflare.TTL == 0 {
			cfg.Cloudflare.TTL = 1
		}
		if IsForbiddenEmail(cfg.TLS.Email) {
			cfg.TLS.Email = ""
		}

		s.data = &cfg
		logger.Info("Loaded configuration from %s (%d proxy targets)", s.configPath, len(s.data.Targets))
		return nil
	}

	// If config.json doesn't exist, initialize clean default config
	logger.Info("No existing config found at %s. Initializing fresh configuration...", s.configPath)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	dir := filepath.Dir(s.configPath)
	tmpFile, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write config to temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to sync temp config file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to close temp config file: %w", err)
	}

	if err := os.Rename(tmpName, s.configPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp config file: %w", err)
	}

	return nil
}

// LegacyTarget represents the shape of proxy_server_old/servers.json
type LegacyServers struct {
	Targets map[string]struct {
		Host         string `json:"host"`
		Scheme       string `json:"scheme"`
		ForceSSL     bool   `json:"force_ssl"`
		RequireAuth  bool   `json:"require_auth"`
		AuthResource string `json:"auth_resource"`
	} `json:"targets"`
}

func (s *Store) ImportLegacyPayloadLocked(content []byte) (int, error) {
	var legacy LegacyServers
	if err := json.Unmarshal(content, &legacy); err != nil {
		return 0, fmt.Errorf("invalid legacy servers.json format: %w", err)
	}

	count := 0
	for domain, t := range legacy.Targets {
		domainClean := strings.ToLower(strings.TrimSpace(domain))
		if domainClean == "" {
			continue
		}

		scheme := strings.ToLower(strings.TrimSpace(t.Scheme))
		if scheme == "" {
			scheme = "http"
		}

		// Check if domain is already in targets, update if so
		var target *ProxyTarget
		for _, existing := range s.data.Targets {
			if strings.ToLower(existing.Domain) == domainClean {
				target = existing
				break
			}
		}

		parts := strings.Split(domainClean, ".")
		name := domainClean
		if len(parts) > 0 && parts[0] != "" {
			name = strings.Title(parts[0]) + " Proxy"
		}

		if target != nil {
			target.Host = t.Host
			target.Scheme = scheme
			target.ForceSSL = t.ForceSSL
			target.RequireAuth = t.RequireAuth
			target.AuthResource = t.AuthResource
			target.UpdatedAt = time.Now()
		} else {
			id := generateID()
			target = &ProxyTarget{
				ID:             id,
				Name:           name,
				Domain:         domainClean,
				Host:           t.Host,
				Scheme:         scheme,
				ForceSSL:       t.ForceSSL,
				RequireAuth:    t.RequireAuth,
				AuthResource:   t.AuthResource,
				PassHostHeader: true,
				Enabled:        true,
				CloudflareSync: true,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}
			s.data.Targets[id] = target
		}
		count++
	}

	logger.Info("Successfully imported %d targets from legacy JSON payload", count)
	return count, nil
}

// GetTargets returns all proxy targets sorted alphabetically by name
func (s *Store) GetTargets() []*ProxyTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targets := make([]*ProxyTarget, 0, len(s.data.Targets))
	for _, t := range s.data.Targets {
		copy := *t
		targets = append(targets, &copy)
	}

	sort.Slice(targets, func(i, j int) bool {
		nameI := strings.ToLower(strings.TrimSpace(targets[i].Name))
		nameJ := strings.ToLower(strings.TrimSpace(targets[j].Name))
		if nameI == nameJ {
			return strings.ToLower(targets[i].Domain) < strings.ToLower(targets[j].Domain)
		}
		return nameI < nameJ
	})

	return targets
}

// GetTargetByID returns a proxy target by ID
func (s *Store) GetTargetByID(id string) (*ProxyTarget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.data.Targets[id]
	if !ok {
		return nil, errors.New("target not found")
	}
	copy := *t
	return &copy, nil
}

// GetTargetByDomain finds a proxy target matching a hostname (case-insensitive)
func (s *Store) GetTargetByDomain(domain string) (*ProxyTarget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	domainLower := strings.ToLower(strings.TrimSpace(domain))
	// 1. Exact match
	for _, t := range s.data.Targets {
		if strings.ToLower(t.Domain) == domainLower && t.Enabled {
			copy := *t
			return &copy, true
		}
	}

	// 2. Wildcard match (e.g. *.home.example.com)
	for _, t := range s.data.Targets {
		if strings.HasPrefix(t.Domain, "*.") && t.Enabled {
			suffix := strings.ToLower(t.Domain[1:]) // e.g. .home.example.com
			if strings.HasSuffix(domainLower, suffix) {
				copy := *t
				return &copy, true
			}
		}
	}

	return nil, false
}

// SaveTarget creates or updates a proxy target
func (s *Store) SaveTarget(target *ProxyTarget) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	domainLower := strings.ToLower(strings.TrimSpace(target.Domain))
	if domainLower == "" {
		return errors.New("domain is required")
	}
	if target.Host == "" {
		return errors.New("target host is required")
	}
	if target.Scheme == "" {
		target.Scheme = "http"
	}

	// Check domain uniqueness across other targets
	for id, existing := range s.data.Targets {
		if id != target.ID && strings.ToLower(existing.Domain) == domainLower {
			return fmt.Errorf("domain %s is already assigned to another proxy target (%s)", domainLower, existing.Name)
		}
	}

	target.Domain = domainLower
	now := time.Now()

	if target.ID == "" {
		target.ID = generateID()
		target.CreatedAt = now
	}
	target.UpdatedAt = now

	s.data.Targets[target.ID] = target

	if err := s.saveLocked(); err != nil {
		return err
	}

	s.notifyChange()
	return nil
}

// DeleteTarget removes a target by ID
func (s *Store) DeleteTarget(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Targets[id]; !ok {
		return errors.New("target not found")
	}

	delete(s.data.Targets, id)

	if err := s.saveLocked(); err != nil {
		return err
	}

	s.notifyChange()
	return nil
}

// ToggleTarget toggles enabled state of a target
func (s *Store) ToggleTarget(id string) (*ProxyTarget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.data.Targets[id]
	if !ok {
		return nil, errors.New("target not found")
	}

	target.Enabled = !target.Enabled
	target.UpdatedAt = time.Now()

	if err := s.saveLocked(); err != nil {
		return nil, err
	}

	copy := *target
	s.notifyChange()
	return &copy, nil
}

// GetCloudflareConfig returns the Cloudflare configuration
func (s *Store) GetCloudflareConfig() CloudflareConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Cloudflare
}

// SaveCloudflareConfig updates the Cloudflare configuration
func (s *Store) SaveCloudflareConfig(cfg CloudflareConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Cloudflare = cfg
	if err := s.saveLocked(); err != nil {
		return err
	}

	s.notifyChange()
	return nil
}

// GetSecurityConfig returns security and admin configuration
func (s *Store) GetSecurityConfig() SecurityConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Security
}

// SaveSecurityConfig updates security settings
func (s *Store) SaveSecurityConfig(cfg SecurityConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Security = cfg
	return s.saveLocked()
}

// GetTLSConfig returns TLS configuration
func (s *Store) GetTLSConfig() TLSConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.data.TLS
	if IsForbiddenEmail(cfg.Email) {
		cfg.Email = ""
	}
	return cfg
}

// SaveTLSConfig updates TLS configuration
func (s *Store) SaveTLSConfig(cfg TLSConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg.Email = strings.TrimSpace(cfg.Email)
	if IsForbiddenEmail(cfg.Email) {
		cfg.Email = ""
	}

	s.data.TLS = cfg
	if err := s.saveLocked(); err != nil {
		return err
	}

	s.notifyChange()
	return nil
}

// ImportLegacyPayload imports targets from a legacy JSON payload string or bytes
func (s *Store) ImportLegacyPayload(content []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := len(s.data.Targets)
	if _, err := s.ImportLegacyPayloadLocked(content); err != nil {
		return 0, err
	}
	after := len(s.data.Targets)

	if err := s.saveLocked(); err != nil {
		return 0, err
	}

	s.notifyChange()
	return after - before, nil
}

// ExportJSON returns the entire configuration as indented JSON
func (s *Store) ExportJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Redact secrets in export
	exportData := *s.data
	exportData.Security.PasswordHash = ""
	exportData.Security.JwtSecret = ""

	return json.MarshalIndent(exportData, "", "  ")
}

// ImportJSON imports external configuration JSON
func (s *Store) ImportJSON(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newCfg Config
	if err := json.Unmarshal(data, &newCfg); err != nil {
		return fmt.Errorf("invalid json format: %w", err)
	}

	// Preserve existing admin password and JWT secret if not provided
	if newCfg.Security.PasswordHash == "" {
		newCfg.Security.PasswordHash = s.data.Security.PasswordHash
		newCfg.Security.AdminUsername = s.data.Security.AdminUsername
		newCfg.Security.IsConfigured = s.data.Security.IsConfigured
	}
	if newCfg.Security.JwtSecret == "" {
		newCfg.Security.JwtSecret = s.data.Security.JwtSecret
	}

	s.data = &newCfg
	if err := s.saveLocked(); err != nil {
		return err
	}

	s.notifyChange()
	return nil
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
