package auth

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	AdminCookieName = "proxyserver_admin_token"
	ProxyCookieName = "proxyserver_session"
)

// AdminClaims contains JWT claims for authenticated admin users
type AdminClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// ProxySessionClaims contains JWT claims for users accessing auth-protected proxies
type ProxySessionClaims struct {
	Username string `json:"username"`
	Resource string `json:"resource,omitempty"`
	jwt.RegisteredClaims
}

// HashPassword hashes a raw password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword verifies a raw password against a bcrypt hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateAdminToken creates a signed JWT for the admin
func GenerateAdminToken(username, secret string, durationHours int) (string, error) {
	if durationHours <= 0 {
		durationHours = 72
	}
	claims := AdminClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "proxyserver",
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationHours) * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateAdminToken parses and validates an admin JWT token
func ValidateAdminToken(tokenStr, secret string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AdminClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*AdminClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// GenerateProxySessionToken creates a token for accessing protected proxy routes
func GenerateProxySessionToken(username, resource, secret string, durationHours int) (string, error) {
	if durationHours <= 0 {
		durationHours = 24
	}
	claims := ProxySessionClaims{
		Username: username,
		Resource: resource,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "proxyserver-gateway",
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(durationHours) * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateProxySessionToken validates a session token for protected proxy routes
func ValidateProxySessionToken(tokenStr, secret string) (*ProxySessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &ProxySessionClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*ProxySessionClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// ExtractToken retrieves JWT from Bearer Authorization header or cookie
func ExtractToken(r *http.Request, cookieName string) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	cookie, err := r.Cookie(cookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// SetAuthCookie sets an HTTP-only secure cookie with the JWT token
func SetAuthCookie(w http.ResponseWriter, cookieName, token string, durationHours int) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(time.Duration(durationHours) * time.Hour),
		MaxAge:   durationHours * 3600,
		HttpOnly: true,
		Secure:   false, // Can be true if running exclusively over HTTPS
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookie deletes the auth cookie
func ClearAuthCookie(w http.ResponseWriter, cookieName string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// RateLimiter manages failed login attempt rates per IP
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a rate limiter with specified max attempts and time window
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}

	// Periodic cleanup of stale entries
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// Allow checks if the given key (e.g. IP) is allowed to perform the action
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.attempts[key] = valid
		return false
	}

	valid = append(valid, now)
	rl.attempts[key] = valid
	return true
}

// Reset clears the attempts for a given key upon successful action
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for key, times := range rl.attempts {
		var valid []time.Time
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.attempts, key)
		} else {
			rl.attempts[key] = valid
		}
	}
}
