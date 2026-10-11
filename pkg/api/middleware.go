package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sebmak/proxyserver/pkg/auth"
	"github.com/sebmak/proxyserver/pkg/store"
)

type contextKey string

const userContextKey contextKey = "admin_user"

// SecurityHeaders adds critical security headers to all responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware allows local development via Vite (e.g. port 5173)
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAdminAuth ensures the request has a valid admin JWT
func RequireAdminAuth(s *store.Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sec := s.GetSecurityConfig()

		// If admin is not configured yet, allow setup endpoints but block others
		if !sec.IsConfigured {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "Initial setup required",
				"code":  "SETUP_REQUIRED",
			})
			return
		}

		tokenStr := auth.ExtractToken(r, auth.AdminCookieName)
		if tokenStr == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "Authentication required",
				"code":  "UNAUTHORIZED",
			})
			return
		}

		claims, err := auth.ValidateAdminToken(tokenStr, sec.JwtSecret)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "Invalid or expired session",
				"code":  "TOKEN_EXPIRED",
			})
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func readJSON(r *http.Request, target any) error {
	return json.NewDecoder(r.Body).Decode(target)
}

func getPathID(r *http.Request, prefix string) string {
	path := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
