package velocity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const RequestIDKey = "velocity.request_id"

// Recovery prevents a panic from taking down the server. The panic payload and
// stack are logged, while the client receives only a generic 500 response.
func Recovery() HandlerFunc { return RecoveryWithLogger(slog.Default()) }

func RecoveryWithLogger(logger *slog.Logger) HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *Context) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "panic", recovered, "method", c.Request.Method, "path", c.Request.URL.Path, "stack", string(debug.Stack()))
				err = &HTTPError{Status: http.StatusInternalServerError, Message: "internal server error", Err: fmt.Errorf("panic: %v", recovered)}
			}
		}()
		return c.Next()
	}
}

// RequestID attaches a validated client-supplied ID or a new random 128-bit ID
// to both context and response. The input restriction prevents header injection.
func RequestID() HandlerFunc {
	return func(c *Context) error {
		id := c.Request.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			id = newRequestID()
		}
		c.Set(RequestIDKey, id)
		c.writer.Header().Set("X-Request-ID", id)
		return c.Next()
	}
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '-' && char != '_' && char != '.' {
			return false
		}
	}
	return true
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	// crypto/rand failures are extraordinary; this fallback remains unique enough
	// for correlation without claiming cryptographic entropy.
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// SecurityHeaders controls browser-facing defensive response headers.
type SecurityHeaders struct {
	ContentSecurityPolicy string
	ReferrerPolicy        string
	PermissionsPolicy     string
	FrameOptions          string
	HSTSMaxAge            time.Duration
	IncludeSubdomains     bool
}

func DefaultSecurityHeaders() SecurityHeaders {
	return SecurityHeaders{
		ContentSecurityPolicy: "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		PermissionsPolicy:     "camera=(), microphone=(), geolocation=()",
		FrameOptions:          "DENY",
	}
}

// SecureHeaders adds conservative headers. HSTS is emitted only on TLS requests
// because sending it over HTTP is ignored by browsers and can surprise local use.
func SecureHeaders(config SecurityHeaders) HandlerFunc {
	if config.ReferrerPolicy == "" {
		config.ReferrerPolicy = "strict-origin-when-cross-origin"
	}
	if config.FrameOptions == "" {
		config.FrameOptions = "DENY"
	}
	return func(c *Context) error {
		headers := c.writer.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", config.FrameOptions)
		headers.Set("Referrer-Policy", config.ReferrerPolicy)
		if config.ContentSecurityPolicy != "" {
			headers.Set("Content-Security-Policy", config.ContentSecurityPolicy)
		}
		if config.PermissionsPolicy != "" {
			headers.Set("Permissions-Policy", config.PermissionsPolicy)
		}
		if c.Request.TLS != nil && config.HSTSMaxAge > 0 {
			value := "max-age=" + strconv.FormatInt(int64(config.HSTSMaxAge.Seconds()), 10)
			if config.IncludeSubdomains {
				value += "; includeSubDomains"
			}
			headers.Set("Strict-Transport-Security", value)
		}
		return c.Next()
	}
}

// MaxBodyBytes limits read request bodies. Install it before any body-binding
// middleware or handler. Content-Length is rejected early when available.
func MaxBodyBytes(limit int64) HandlerFunc {
	if limit <= 0 {
		panic("velocity: body limit must be positive")
	}
	return func(c *Context) error {
		if c.Request.ContentLength > limit {
			return ErrPayloadTooLarge
		}
		c.Request.Body = http.MaxBytesReader(c.Writer(), c.Request.Body, limit)
		return c.Next()
	}
}

// CORSConfig intentionally requires explicit origins. Never combine a wildcard
// origin with credentials; browsers reject it and it is unsafe by design.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           time.Duration
}

func CORS(config CORSConfig) HandlerFunc {
	if len(config.AllowedOrigins) == 0 {
		panic("velocity: CORS requires at least one allowed origin")
	}
	if len(config.AllowedMethods) == 0 {
		config.AllowedMethods = []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete}
	}
	if config.AllowCredentials {
		for _, origin := range config.AllowedOrigins {
			if origin == "*" {
				panic("velocity: wildcard CORS origin cannot allow credentials")
			}
		}
	}
	allowedMethods := strings.Join(config.AllowedMethods, ", ")
	allowedHeaders := strings.Join(config.AllowedHeaders, ", ")
	exposedHeaders := strings.Join(config.ExposedHeaders, ", ")
	return func(c *Context) error {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			return c.Next()
		}
		matched := false
		wildcard := false
		for _, allowed := range config.AllowedOrigins {
			if allowed == "*" {
				matched, wildcard = true, true
				break
			}
			if subtle.ConstantTimeCompare([]byte(origin), []byte(allowed)) == 1 {
				matched = true
				break
			}
		}
		if !matched {
			return c.Next()
		}
		headers := c.writer.Header()
		headers.Add("Vary", "Origin")
		if wildcard && !config.AllowCredentials {
			headers.Set("Access-Control-Allow-Origin", "*")
		} else {
			headers.Set("Access-Control-Allow-Origin", origin)
		}
		if config.AllowCredentials {
			headers.Set("Access-Control-Allow-Credentials", "true")
		}
		if exposedHeaders != "" {
			headers.Set("Access-Control-Expose-Headers", exposedHeaders)
		}
		if c.Request.Method == http.MethodOptions && c.Request.Header.Get("Access-Control-Request-Method") != "" {
			requestedMethod := c.Request.Header.Get("Access-Control-Request-Method")
			if !containsFold(config.AllowedMethods, requestedMethod) {
				return c.Next()
			}
			requestedHeaders := strings.Split(c.Request.Header.Get("Access-Control-Request-Headers"), ",")
			for _, requestedHeader := range requestedHeaders {
				requestedHeader = strings.TrimSpace(requestedHeader)
				if requestedHeader != "" && !containsFold(config.AllowedHeaders, requestedHeader) {
					return ErrForbidden
				}
			}
			headers.Set("Access-Control-Allow-Methods", allowedMethods)
			if allowedHeaders != "" {
				headers.Set("Access-Control-Allow-Headers", allowedHeaders)
			}
			if config.MaxAge > 0 {
				headers.Set("Access-Control-Max-Age", strconv.FormatInt(int64(config.MaxAge.Seconds()), 10))
			}
			c.AbortWithStatus(http.StatusNoContent)
			return nil
		}
		return c.Next()
	}
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

// RateLimitConfig configures a bounded in-memory token-bucket limiter. For a
// multi-instance deployment, use the Key function with a shared edge limiter.
type RateLimitConfig struct {
	Rate       float64
	Burst      int
	MaxEntries int
	Key        func(*Context) string
}

type rateBucket struct {
	tokens  float64
	updated time.Time
}

// RateLimit returns 429 with Retry-After when a key's bucket is empty.
func RateLimit(config RateLimitConfig) HandlerFunc {
	if config.Rate <= 0 || config.Burst <= 0 {
		panic("velocity: rate and burst must be positive")
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = 10000
	}
	if config.Key == nil {
		config.Key = func(c *Context) string { return c.ClientIP() }
	}
	var lock sync.Mutex
	buckets := make(map[string]rateBucket)
	return func(c *Context) error {
		key := config.Key(c)
		if key == "" {
			key = "anonymous"
		}
		now := time.Now()
		lock.Lock()
		bucket, exists := buckets[key]
		if !exists {
			if len(buckets) >= config.MaxEntries {
				for candidate, value := range buckets {
					if now.Sub(value.updated) > time.Minute {
						delete(buckets, candidate)
					}
				}
				if len(buckets) >= config.MaxEntries {
					lock.Unlock()
					return NewHTTPError(http.StatusTooManyRequests, "rate limit capacity reached")
				}
			}
			bucket = rateBucket{tokens: float64(config.Burst), updated: now}
		}
		elapsed := now.Sub(bucket.updated).Seconds()
		bucket.tokens = minFloat(float64(config.Burst), bucket.tokens+elapsed*config.Rate)
		bucket.updated = now
		if bucket.tokens < 1 {
			buckets[key] = bucket
			wait := (1 - bucket.tokens) / config.Rate
			lock.Unlock()
			c.writer.Header().Set("Retry-After", strconv.Itoa(maxInt(1, int(wait+0.999))))
			return &HTTPError{Status: http.StatusTooManyRequests, Code: "RATE_LIMITED", Message: "rate limit exceeded"}
		}
		bucket.tokens--
		buckets[key] = bucket
		lock.Unlock()
		return c.Next()
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// BasicAuth authenticates fixed credentials with constant-time hash comparison.
// Use it for simple administrative endpoints, not as a substitute for a full
// identity system where users or credentials change frequently.
func BasicAuth(credentials map[string]string, realm string) HandlerFunc {
	if len(credentials) == 0 {
		panic("velocity: BasicAuth requires credentials")
	}
	if realm == "" {
		realm = "restricted"
	}
	copyCredentials := make(map[string][32]byte, len(credentials))
	for username, password := range credentials {
		copyCredentials[username] = sha256.Sum256([]byte(password))
	}
	return func(c *Context) error {
		username, password, ok := c.Request.BasicAuth()
		expected, known := copyCredentials[username]
		actual := sha256.Sum256([]byte(password))
		if !ok || !known || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			c.writer.Header().Set("WWW-Authenticate", `Basic realm="`+strings.ReplaceAll(realm, `"`, "")+`", charset="UTF-8"`)
			return ErrUnauthorized
		}
		c.Set("velocity.basic_auth_user", username)
		return c.Next()
	}
}
