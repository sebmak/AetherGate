package proxy

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sebmak/proxyserver/pkg/auth"
	"github.com/sebmak/proxyserver/pkg/logger"
	"github.com/sebmak/proxyserver/pkg/store"
)

// Metrics holds atomic counters for reverse proxy traffic
type Metrics struct {
	TotalRequests atomic.Uint64 `json:"total_requests"`
	Status2xx     atomic.Uint64 `json:"status_2xx"`
	Status3xx     atomic.Uint64 `json:"status_3xx"`
	Status4xx     atomic.Uint64 `json:"status_4xx"`
	Status5xx     atomic.Uint64 `json:"status_5xx"`
	TotalBytes    atomic.Uint64 `json:"total_bytes"`
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	TotalRequests uint64 `json:"total_requests"`
	Status2xx     uint64 `json:"status_2xx"`
	Status3xx     uint64 `json:"status_3xx"`
	Status4xx     uint64 `json:"status_4xx"`
	Status5xx     uint64 `json:"status_5xx"`
	TotalBytes    uint64 `json:"total_bytes"`
}

// GetSnapshot returns current metrics as a plain struct
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRequests: m.TotalRequests.Load(),
		Status2xx:     m.Status2xx.Load(),
		Status3xx:     m.Status3xx.Load(),
		Status4xx:     m.Status4xx.Load(),
		Status5xx:     m.Status5xx.Load(),
		TotalBytes:    m.TotalBytes.Load(),
	}
}

// Engine executes dynamic reverse proxying with live routing and metrics
type Engine struct {
	store         *store.Store
	healthChecker *HealthChecker
	metrics       *Metrics
	transport     *http.Transport
}

// NewEngine creates an Engine instance
func NewEngine(s *store.Store, hc *HealthChecker) *Engine {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          500,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Allow upstream internal self-signed HTTPS
		},
	}

	return &Engine{
		store:         s,
		healthChecker: hc,
		metrics:       &Metrics{},
		transport:     transport,
	}
}

// GetMetrics returns the traffic metrics
func (e *Engine) GetMetrics() *Metrics {
	return e.metrics
}

// GetHealthChecker returns the upstream health checker
func (e *Engine) GetHealthChecker() *HealthChecker {
	return e.healthChecker
}

// ServeHTTP handles all incoming reverse proxy traffic
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	e.metrics.TotalRequests.Add(1)

	// Clean host header (remove port if present)
	host := r.Host
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			host = h
		}
	}

	// 1. Look up target dynamically in store
	target, found := e.store.GetTargetByDomain(host)
	if !found {
		e.metrics.Status4xx.Add(1)
		e.serveNotFound(w, r, host)
		return
	}

	// 2. Check ForceSSL redirect
	isTLS := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	if target.ForceSSL && !isTLS {
		e.metrics.Status3xx.Add(1)
		targetURL := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, targetURL, http.StatusMovedPermanently)
		return
	}

	// 3. Check Authentication if required
	if target.RequireAuth {
		if !e.isAuthenticated(r, target) {
			if r.URL.Path == "/_auth/login" {
				e.handleProxyLogin(w, r, target)
				return
			}
			e.serveAuthGate(w, r, target)
			return
		}
	}

	// 4. Build reverse proxy to upstream
	upstreamURL, err := url.Parse(fmt.Sprintf("%s://%s", target.Scheme, target.Host))
	if err != nil {
		e.metrics.Status5xx.Add(1)
		http.Error(w, "Invalid upstream configuration", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	proxy.Transport = e.transport

	// Custom Director to inject headers and handle WebSockets/Host
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Set target host & scheme
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		if target.PassHostHeader {
			req.Host = r.Host
		} else {
			req.Host = target.Host
		}

		// Client IP tracking
		clientIP := extractClientIP(r)
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			clientIP = prior + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Real-IP", extractClientIP(r))
		req.Header.Set("X-Forwarded-Host", r.Host)

		proto := "http"
		if isTLS {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)

		// Apply custom headers
		for k, v := range target.CustomHeaders {
			req.Header.Set(k, v)
		}
	}

	// Response Modifier for telemetry & logs
	proxy.ModifyResponse = func(resp *http.Response) error {
		duration := time.Since(start).Milliseconds()
		code := resp.StatusCode

		if code >= 200 && code < 300 {
			e.metrics.Status2xx.Add(1)
		} else if code >= 300 && code < 400 {
			e.metrics.Status3xx.Add(1)
		} else if code >= 400 && code < 500 {
			e.metrics.Status4xx.Add(1)
		} else if code >= 500 {
			e.metrics.Status5xx.Add(1)
		}

		if resp.ContentLength > 0 {
			e.metrics.TotalBytes.Add(uint64(resp.ContentLength))
		}

		logger.GetLogBuffer().Add(&logger.RequestLog{
			Timestamp:  start,
			ClientIP:   extractClientIP(r),
			Method:     r.Method,
			Host:       r.Host,
			Path:       r.URL.Path,
			Target:     target.Host,
			StatusCode: code,
			DurationMs: duration,
			BytesSent:  resp.ContentLength,
		})

		return nil
	}

	// Error handler for unreachable upstreams
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		e.metrics.Status5xx.Add(1)
		duration := time.Since(start).Milliseconds()

		logger.GetLogBuffer().Add(&logger.RequestLog{
			Timestamp:  start,
			ClientIP:   extractClientIP(r),
			Method:     r.Method,
			Host:       r.Host,
			Path:       r.URL.Path,
			Target:     target.Host,
			StatusCode: http.StatusBadGateway,
			DurationMs: duration,
			Error:      proxyErr.Error(),
		})

		e.serveBadGateway(rw, req, target, proxyErr)
	}

	proxy.ServeHTTP(w, r)
}

func (e *Engine) isAuthenticated(r *http.Request, target *store.ProxyTarget) bool {
	sec := e.store.GetSecurityConfig()
	secret := sec.JwtSecret

	// Check proxy session cookie
	tokenStr := auth.ExtractToken(r, auth.ProxyCookieName)
	if tokenStr != "" {
		if _, err := auth.ValidateProxySessionToken(tokenStr, secret); err == nil {
			return true
		}
	}

	// Also allow logged-in admin access
	adminToken := auth.ExtractToken(r, auth.AdminCookieName)
	if adminToken != "" {
		if _, err := auth.ValidateAdminToken(adminToken, secret); err == nil {
			return true
		}
	}

	return false
}

func (e *Engine) handleProxyLogin(w http.ResponseWriter, r *http.Request, target *store.ProxyTarget) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	redirectURL := r.FormValue("redirect")
	if redirectURL == "" {
		redirectURL = "/"
	}

	sec := e.store.GetSecurityConfig()
	if username == sec.AdminUsername && auth.CheckPassword(password, sec.PasswordHash) {
		token, err := auth.GenerateProxySessionToken(username, target.Domain, sec.JwtSecret, 24)
		if err == nil {
			auth.SetAuthCookie(w, auth.ProxyCookieName, token, 24)
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
	}

	// Failed login
	w.WriteHeader(http.StatusUnauthorized)
	e.renderAuthPage(w, target, redirectURL, "Invalid username or password")
}

func (e *Engine) serveAuthGate(w http.ResponseWriter, r *http.Request, target *store.ProxyTarget) {
	w.WriteHeader(http.StatusUnauthorized)
	e.renderAuthPage(w, target, r.URL.RequestURI(), "")
}

func (e *Engine) renderAuthPage(w http.ResponseWriter, target *store.ProxyTarget, redirectURL, errMsg string) {
	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Authentication Required - {{.TargetName}}</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #090d16;
      color: #e2e8f0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 1.5rem;
    }
    .card {
      background: rgba(15, 23, 42, 0.85);
      border: 1px solid rgba(255, 255, 255, 0.1);
      border-radius: 1rem;
      backdrop-filter: blur(16px);
      box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
      width: 100%;
      max-width: 400px;
      padding: 2.5rem;
    }
    .badge {
      display: inline-block;
      padding: 0.25rem 0.75rem;
      font-size: 0.75rem;
      font-weight: 600;
      border-radius: 9999px;
      background: rgba(59, 130, 246, 0.15);
      color: #60a5fa;
      border: 1px solid rgba(59, 130, 246, 0.3);
      margin-bottom: 1rem;
    }
    h1 { font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem; color: #fff; }
    p.desc { font-size: 0.875rem; color: #94a3b8; margin-bottom: 1.5rem; }
    .error {
      background: rgba(239, 68, 68, 0.15);
      border: 1px solid rgba(239, 68, 68, 0.3);
      color: #fca5a5;
      padding: 0.75rem;
      border-radius: 0.5rem;
      font-size: 0.875rem;
      margin-bottom: 1.25rem;
    }
    .form-group { margin-bottom: 1.25rem; text-align: left; }
    label { display: block; font-size: 0.8125rem; font-weight: 500; color: #cbd5e1; margin-bottom: 0.5rem; }
    input {
      width: 100%;
      background: #1e293b;
      border: 1px solid #334155;
      border-radius: 0.5rem;
      color: #fff;
      padding: 0.75rem 1rem;
      font-size: 0.9375rem;
      outline: none;
      transition: border-color 0.2s;
    }
    input:focus { border-color: #3b82f6; }
    button {
      width: 100%;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      color: #fff;
      border: none;
      border-radius: 0.5rem;
      padding: 0.75rem;
      font-size: 0.9375rem;
      font-weight: 600;
      cursor: pointer;
      box-shadow: 0 4px 12px rgba(37, 99, 235, 0.3);
      transition: opacity 0.2s;
    }
    button:hover { opacity: 0.9; }
  </style>
</head>
<body>
  <div class="card">
    <div class="badge">SECURE GATEWAY</div>
    <h1>{{.TargetName}}</h1>
    <p class="desc">Authentication required to access <strong>{{.Domain}}</strong></p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="POST" action="/_auth/login">
      <input type="hidden" name="redirect" value="{{.RedirectURL}}">
      <div class="form-group">
        <label for="username">Username</label>
        <input type="text" id="username" name="username" required autofocus placeholder="Admin username">
      </div>
      <div class="form-group">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" required placeholder="••••••••">
      </div>
      <button type="submit">Sign In to Continue</button>
    </form>
  </div>
</body>
</html>`

	t := template.Must(template.New("auth").Parse(tmpl))
	_ = t.Execute(w, map[string]any{
		"TargetName":  target.Name,
		"Domain":      target.Domain,
		"RedirectURL": redirectURL,
		"Error":       errMsg,
	})
}

func (e *Engine) serveNotFound(w http.ResponseWriter, r *http.Request, host string) {
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>404 - Domain Not Configured</title>
  <style>
    body { background: #090d16; color: #e2e8f0; font-family: system-ui, sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; text-align: center; }
    .card { background: #0f172a; border: 1px solid #1e293b; border-radius: 12px; padding: 2.5rem; max-width: 480px; box-shadow: 0 20px 40px rgba(0,0,0,0.5); }
    h1 { color: #f87171; margin-bottom: 0.5rem; font-size: 1.75rem; }
    p { color: #94a3b8; font-size: 0.95rem; line-height: 1.5; margin-bottom: 1.5rem; }
    code { background: #1e293b; color: #38bdf8; padding: 0.2rem 0.5rem; border-radius: 4px; font-size: 0.9rem; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Domain Not Configured</h1>
    <p>The hostname <code>%s</code> is reaching this proxy server, but no active proxy route is configured for it.</p>
    <p>Please log in to the management dashboard to register a new proxy target for this domain.</p>
  </div>
</body>
</html>`, template.HTMLEscapeString(host))
	_, _ = w.Write([]byte(html))
}

func (e *Engine) serveBadGateway(w http.ResponseWriter, r *http.Request, target *store.ProxyTarget, proxyErr error) {
	w.WriteHeader(http.StatusBadGateway)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>502 - Upstream Unreachable</title>
  <style>
    body { background: #090d16; color: #e2e8f0; font-family: system-ui, sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; text-align: center; }
    .card { background: #0f172a; border: 1px solid #1e293b; border-radius: 12px; padding: 2.5rem; max-width: 500px; box-shadow: 0 20px 40px rgba(0,0,0,0.5); }
    h1 { color: #fbbf24; margin-bottom: 0.5rem; font-size: 1.75rem; }
    p { color: #94a3b8; font-size: 0.95rem; line-height: 1.5; margin-bottom: 1.25rem; }
    .info { background: #1e293b; border-radius: 8px; padding: 1rem; text-align: left; font-family: monospace; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 1rem; overflow-x: auto; }
  </style>
</head>
<body>
  <div class="card">
    <h1>502 Bad Gateway</h1>
    <p>Could not connect to the upstream target for <strong>%s</strong>.</p>
    <div class="info">
      <div><strong>Target:</strong> %s://%s</div>
      <div><strong>Error:</strong> %s</div>
    </div>
    <p>Verify that the upstream service is running on your network and accepting connections.</p>
  </div>
</body>
</html>`, template.HTMLEscapeString(target.Domain), template.HTMLEscapeString(target.Scheme), template.HTMLEscapeString(target.Host), template.HTMLEscapeString(proxyErr.Error()))
	_, _ = w.Write([]byte(html))
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
