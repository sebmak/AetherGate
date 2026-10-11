package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sebmak/proxyserver/pkg/auth"
	"github.com/sebmak/proxyserver/pkg/config"
	"github.com/sebmak/proxyserver/pkg/ddns"
	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/proxy"
	"github.com/sebmak/proxyserver/pkg/store"
)

// Server handles the Management API and Web UI serving
type Server struct {
	cfg         *config.AppConfig
	store       *store.Store
	proxyEngine *proxy.Engine
	ddnsManager *ddns.Manager
	rateLimiter *auth.RateLimiter
	mux         *http.ServeMux
	startTime   time.Time
}

// NewServer creates a new API Server
func NewServer(cfg *config.AppConfig, s *store.Store, pe *proxy.Engine, dm *ddns.Manager) *Server {
	srv := &Server{
		cfg:         cfg,
		store:       s,
		proxyEngine: pe,
		ddnsManager: dm,
		rateLimiter: auth.NewRateLimiter(5, 1*time.Minute),
		mux:         http.NewServeMux(),
		startTime:   time.Now(),
	}

	srv.registerRoutes()
	return srv
}

// Handler returns the HTTP handler with middleware
func (s *Server) Handler(staticFS http.Handler) http.Handler {
	// If a static files handler is provided, route non-API requests to it
	var rootHandler http.Handler = s.mux
	if staticFS != nil {
		rootHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				s.mux.ServeHTTP(w, r)
				return
			}
			staticFS.ServeHTTP(w, r)
		})
	}

	return CORSMiddleware(SecurityHeaders(rootHandler))
}

func (s *Server) registerRoutes() {
	// Setup & Status
	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)

	// Auth
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/auth/me", RequireAdminAuth(s.store, s.handleMe))

	// Health & System
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/stats", RequireAdminAuth(s.store, s.handleStats))

	// Proxies
	s.mux.HandleFunc("GET /api/proxies", RequireAdminAuth(s.store, s.handleGetProxies))
	s.mux.HandleFunc("POST /api/proxies", RequireAdminAuth(s.store, s.handleCreateProxy))
	s.mux.HandleFunc("GET /api/proxies/{id}", RequireAdminAuth(s.store, s.handleGetProxy))
	s.mux.HandleFunc("PUT /api/proxies/{id}", RequireAdminAuth(s.store, s.handleUpdateProxy))
	s.mux.HandleFunc("DELETE /api/proxies/{id}", RequireAdminAuth(s.store, s.handleDeleteProxy))
	s.mux.HandleFunc("POST /api/proxies/{id}/toggle", RequireAdminAuth(s.store, s.handleToggleProxy))
	s.mux.HandleFunc("POST /api/proxies/test-upstream", RequireAdminAuth(s.store, s.handleTestUpstream))

	// Cloudflare DDNS
	s.mux.HandleFunc("GET /api/ddns/status", RequireAdminAuth(s.store, s.handleDDNSStatus))
	s.mux.HandleFunc("POST /api/ddns/config", RequireAdminAuth(s.store, s.handleDDNSConfig))
	s.mux.HandleFunc("POST /api/ddns/sync", RequireAdminAuth(s.store, s.handleDDNSSync))
	s.mux.HandleFunc("POST /api/ddns/test-token", RequireAdminAuth(s.store, s.handleDDNSTestToken))

	// Logs
	s.mux.HandleFunc("GET /api/logs", RequireAdminAuth(s.store, s.handleGetLogs))
	s.mux.HandleFunc("GET /api/logs/stream", RequireAdminAuth(s.store, s.handleStreamLogs))

	// Settings
	s.mux.HandleFunc("GET /api/settings", RequireAdminAuth(s.store, s.handleGetSettings))
	s.mux.HandleFunc("PUT /api/settings", RequireAdminAuth(s.store, s.handleUpdateSettings))
	s.mux.HandleFunc("POST /api/settings/change-password", RequireAdminAuth(s.store, s.handleChangePassword))
	s.mux.HandleFunc("GET /api/settings/export", RequireAdminAuth(s.store, s.handleExportConfig))
	s.mux.HandleFunc("POST /api/settings/import", RequireAdminAuth(s.store, s.handleImportConfig))
	s.mux.HandleFunc("POST /api/settings/import-legacy", RequireAdminAuth(s.store, s.handleImportLegacy))
}

// Setup handlers
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	sec := s.store.GetSecurityConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"is_configured": sec.IsConfigured,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	sec := s.store.GetSecurityConfig()
	if sec.IsConfigured {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Initial setup has already been completed"})
		return
	}

	var req struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		LegacyConfig string `json:"legacy_config,omitempty"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Username is required and password must be at least 6 characters"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to hash password"})
		return
	}

	sec.AdminUsername = req.Username
	sec.PasswordHash = hash
	sec.IsConfigured = true

	if err := s.store.SaveSecurityConfig(sec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to save security configuration"})
		return
	}

	// If user provided a legacy configuration JSON during setup, import it
	importedCount := 0
	if trimmedLegacy := strings.TrimSpace(req.LegacyConfig); trimmedLegacy != "" {
		count, importErr := s.store.ImportLegacyPayload([]byte(trimmedLegacy))
		if importErr != nil {
			logger.Warn("Failed to import pasted legacy config during setup: %v", importErr)
		} else {
			importedCount = count
			logger.Info("Imported %d targets from pasted legacy config during setup", count)
		}
	}

	token, err := auth.GenerateAdminToken(sec.AdminUsername, sec.JwtSecret, sec.SessionDurationHours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to generate token"})
		return
	}

	auth.SetAuthCookie(w, auth.AdminCookieName, token, sec.SessionDurationHours)
	logger.Info("Admin account initialized successfully for user '%s'", sec.AdminUsername)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"token":          token,
		"username":       sec.AdminUsername,
		"imported_count": importedCount,
	})
}

// Auth handlers
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr
	if !s.rateLimiter.Allow(clientIP) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "Too many failed attempts. Please wait 1 minute."})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request format"})
		return
	}

	sec := s.store.GetSecurityConfig()
	if !sec.IsConfigured {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Admin setup is not yet completed"})
		return
	}

	if req.Username != sec.AdminUsername || !auth.CheckPassword(req.Password, sec.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Invalid username or password"})
		return
	}

	s.rateLimiter.Reset(clientIP)

	token, err := auth.GenerateAdminToken(sec.AdminUsername, sec.JwtSecret, sec.SessionDurationHours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to generate session token"})
		return
	}

	auth.SetAuthCookie(w, auth.AdminCookieName, token, sec.SessionDurationHours)
	logger.Info("Successful admin login by '%s' from %s", sec.AdminUsername, clientIP)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"token":    token,
		"username": sec.AdminUsername,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearAuthCookie(w, auth.AdminCookieName)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	username, _ := r.Context().Value(userContextKey).(string)
	writeJSON(w, http.StatusOK, map[string]any{
		"username": username,
	})
}

// Health & Stats
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"uptime_s":  int64(time.Since(s.startTime).Seconds()),
		"timestamp": time.Now(),
		"version":   config.Version,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	metrics := s.proxyEngine.GetMetrics().GetSnapshot()
	healthMap := s.proxyEngine.GetHealthChecker().GetHealth()

	healthyCount := 0
	totalUpstreams := len(healthMap)
	for _, h := range healthMap {
		if h.IsHealthy {
			healthyCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metrics":         metrics,
		"healthy_targets": healthyCount,
		"total_targets":   totalUpstreams,
		"uptime_seconds":  int64(time.Since(s.startTime).Seconds()),
	})
}

// Proxies handlers
type ProxyTargetWithHealth struct {
	*store.ProxyTarget
	IsHealthy   bool      `json:"is_healthy"`
	LatencyMs   int64     `json:"latency_ms"`
	LastChecked time.Time `json:"last_checked"`
	LastError   string    `json:"last_error,omitempty"`
}

func (s *Server) handleGetProxies(w http.ResponseWriter, r *http.Request) {
	targets := s.store.GetTargets()
	healthMap := s.proxyEngine.GetHealthChecker().GetHealth()

	res := make([]ProxyTargetWithHealth, len(targets))
	for i, t := range targets {
		item := ProxyTargetWithHealth{
			ProxyTarget: t,
		}
		if h, ok := healthMap[t.ID]; ok {
			item.IsHealthy = h.IsHealthy
			item.LatencyMs = h.LatencyMs
			item.LastChecked = h.LastChecked
			item.LastError = h.LastError
		}
		res[i] = item
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCreateProxy(w http.ResponseWriter, r *http.Request) {
	var target store.ProxyTarget
	if err := readJSON(r, &target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	if target.Domain == "" || target.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Domain and Host are required"})
		return
	}

	if target.Scheme == "" {
		target.Scheme = "http"
	}
	if target.Name == "" {
		target.Name = target.Domain
	}

	if err := s.store.SaveTarget(&target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// If Cloudflare DDNS auto-sync is enabled for this target, trigger sync
	if target.CloudflareSync {
		go func(domain string) {
			_ = s.ddnsManager.EnsureDomain(domain)
		}(target.Domain)
	}

	logger.Info("Created new proxy target '%s' for domain '%s' -> %s://%s", target.Name, target.Domain, target.Scheme, target.Host)
	writeJSON(w, http.StatusCreated, target)
}

func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := s.store.GetTargetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Proxy not found"})
		return
	}

	writeJSON(w, http.StatusOK, target)
}

func (s *Server) handleUpdateProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := s.store.GetTargetByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Proxy not found"})
		return
	}

	var req store.ProxyTarget
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	target.Name = req.Name
	target.Domain = req.Domain
	target.Host = req.Host
	target.Scheme = req.Scheme
	target.ForceSSL = req.ForceSSL
	target.RequireAuth = req.RequireAuth
	target.AuthResource = req.AuthResource
	target.CustomHeaders = req.CustomHeaders
	target.PassHostHeader = req.PassHostHeader
	target.Enabled = req.Enabled
	target.CloudflareSync = req.CloudflareSync

	if err := s.store.SaveTarget(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	if target.CloudflareSync && target.Enabled {
		go func(domain string) {
			_ = s.ddnsManager.EnsureDomain(domain)
		}(target.Domain)
	}

	logger.Info("Updated proxy target '%s' (%s)", target.Name, target.Domain)
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) handleDeleteProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteTarget(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	logger.Info("Deleted proxy target with ID %s", id)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleToggleProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := s.store.ToggleTarget(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	logger.Info("Toggled proxy target '%s' (Enabled: %v)", target.Name, target.Enabled)
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) handleTestUpstream(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scheme string `json:"scheme"`
		Host   string `json:"host"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	ok, latency, msg := s.proxyEngine.GetHealthChecker().TestUpstream(req.Scheme, req.Host)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":    ok,
		"latency_ms": latency,
		"message":    msg,
	})
}

// Cloudflare DDNS handlers
func (s *Server) handleDDNSStatus(w http.ResponseWriter, r *http.Request) {
	status := s.ddnsManager.GetStatus()
	cfg := s.store.GetCloudflareConfig()

	// Redact API keys for security in UI payload
	cfgRedacted := cfg
	if cfgRedacted.ApiToken != "" {
		cfgRedacted.ApiToken = maskSecret(cfgRedacted.ApiToken)
	}
	if cfgRedacted.ApiKey != "" {
		cfgRedacted.ApiKey = maskSecret(cfgRedacted.ApiKey)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"config": cfgRedacted,
	})
}

func (s *Server) handleDDNSConfig(w http.ResponseWriter, r *http.Request) {
	var req store.CloudflareConfig
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	current := s.store.GetCloudflareConfig()
	// If submitted token is masked or empty, retain current token
	if strings.HasPrefix(req.ApiToken, "***") || req.ApiToken == "" {
		req.ApiToken = current.ApiToken
	}
	if strings.HasPrefix(req.ApiKey, "***") || req.ApiKey == "" {
		req.ApiKey = current.ApiKey
	}

	if err := s.store.SaveCloudflareConfig(req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	if req.Enabled {
		s.ddnsManager.TriggerSyncNow()
	}

	logger.Info("Updated Cloudflare DDNS configuration (Enabled: %v)", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleDDNSSync(w http.ResponseWriter, r *http.Request) {
	go s.ddnsManager.TriggerSyncNow()
	writeJSON(w, http.StatusOK, map[string]any{"message": "DDNS sync triggered"})
}

func (s *Server) handleDDNSTestToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ApiToken string `json:"api_token"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	current := s.store.GetCloudflareConfig()
	token := strings.TrimSpace(req.ApiToken)
	if token == "" || strings.HasPrefix(token, "***") {
		token = strings.TrimSpace(current.ApiToken)
	}

	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "API Token is required"})
		return
	}

	testCfg := store.CloudflareConfig{ApiToken: token}

	// 1. Verify token status via /user/tokens/verify
	valid, verifyErr := s.ddnsManager.VerifyToken(r.Context(), token)

	// 2. Also check if we can query zones
	zones, zoneErr := s.ddnsManager.ListZones(r.Context(), testCfg)

	if !valid && zoneErr != nil {
		// Both verification methods failed: return detailed descriptive error
		errMsg := "Token verification failed"
		if verifyErr != nil {
			errMsg = fmt.Sprintf("Token verification failed: %s", verifyErr.Error())
		} else if zoneErr != nil {
			errMsg = fmt.Sprintf("Token verification failed: %s", zoneErr.Error())
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"error": errMsg,
		})
		return
	}

	zoneList := zones
	if zoneList == nil {
		zoneList = []ddns.CFZone{}
	}

	var warningMsg string
	if zoneErr != nil {
		warningMsg = fmt.Sprintf("Token verified, but could not list zones: %v", zoneErr)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":   true,
		"zones":   zoneList,
		"warning": warningMsg,
	})
}

// Logs handlers
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	logs := logger.GetLogBuffer().GetRecent(limit)
	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := logger.GetLogBuffer().Subscribe()
	defer logger.GetLogBuffer().Unsubscribe(ch)

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", entry.String())
			flusher.Flush()
		}
	}
}

// Settings handlers
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	sec := s.store.GetSecurityConfig()
	tlsCfg := s.store.GetTLSConfig()

	writeJSON(w, http.StatusOK, map[string]any{
		"admin_username": sec.AdminUsername,
		"admin_port":     s.cfg.AdminPort,
		"http_port":      s.cfg.HTTPPort,
		"https_port":     s.cfg.HTTPSPort,
		"data_dir":       s.cfg.DataDir,
		"tls":            tlsCfg,
	})
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TLS store.TLSConfig `json:"tls"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	if err := s.store.SaveTLSConfig(req.TLS); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid request body"})
		return
	}

	if len(req.NewPassword) < 6 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "New password must be at least 6 characters"})
		return
	}

	sec := s.store.GetSecurityConfig()
	if !auth.CheckPassword(req.CurrentPassword, sec.PasswordHash) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Current password is incorrect"})
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to hash password"})
		return
	}

	sec.PasswordHash = hash
	if err := s.store.SaveSecurityConfig(sec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Failed to save password"})
		return
	}

	logger.Info("Admin password changed successfully")
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleExportConfig(w http.ResponseWriter, r *http.Request) {
	data, err := s.store.ExportJSON()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=proxyserver-config.json")
	_, _ = w.Write(data)
}

func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Failed to read request body"})
		return
	}

	if err := s.store.ImportJSON(body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	logger.Info("Imported configuration from JSON payload")
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) handleImportLegacy(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Failed to read request body"})
		return
	}

	var wrapper struct {
		Payload string `json:"payload"`
	}
	var payloadBytes []byte
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Payload != "" {
		payloadBytes = []byte(wrapper.Payload)
	} else {
		payloadBytes = body
	}

	count, err := s.store.ImportLegacyPayload(payloadBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"imported_count": count,
	})
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
