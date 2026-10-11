package logger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// RequestLog represents a reverse proxy access log entry
type RequestLog struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	ClientIP   string    `json:"client_ip"`
	Method     string    `json:"method"`
	Host       string    `json:"host"`
	Path       string    `json:"path"`
	Target     string    `json:"target"`
	StatusCode int       `json:"status_code"`
	DurationMs int64     `json:"duration_ms"`
	BytesSent  int64     `json:"bytes_sent"`
	Error      string    `json:"error,omitempty"`
}

// SystemLog represents an internal system, TLS, ACME, DDNS, or engine log entry
type SystemLog struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`  // "INFO", "WARN", "ERROR", "DEBUG"
	Source    string    `json:"source"` // "TLS", "ACME", "DDNS", "PROXY", "AUTH", "SYSTEM"
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
}

// LogBuffer maintains an in-memory ring buffer of recent HTTP request logs and broadcasts to subscribers
type LogBuffer struct {
	mu          sync.RWMutex
	logs        []*RequestLog
	maxCapacity int
	subscribers map[chan *RequestLog]struct{}
}

// SystemLogBuffer maintains an in-memory ring buffer of internal system logs and broadcasts to subscribers
type SystemLogBuffer struct {
	mu          sync.RWMutex
	logs        []*SystemLog
	maxCapacity int
	subscribers map[chan *SystemLog]struct{}
}

var (
	defaultBuffer       *LogBuffer
	defaultSystemBuffer *SystemLogBuffer
	once                sync.Once
	systemOnce          sync.Once
)

// GetLogBuffer returns the singleton request log buffer instance
func GetLogBuffer() *LogBuffer {
	once.Do(func() {
		defaultBuffer = &LogBuffer{
			logs:        make([]*RequestLog, 0, 500),
			maxCapacity: 500,
			subscribers: make(map[chan *RequestLog]struct{}),
		}
	})
	return defaultBuffer
}

// GetSystemLogBuffer returns the singleton system log buffer instance
func GetSystemLogBuffer() *SystemLogBuffer {
	systemOnce.Do(func() {
		defaultSystemBuffer = &SystemLogBuffer{
			logs:        make([]*SystemLog, 0, 1000),
			maxCapacity: 1000,
			subscribers: make(map[chan *SystemLog]struct{}),
		}
	})
	return defaultSystemBuffer
}

// Add appends a new request log to the ring buffer and notifies subscribers
func (b *LogBuffer) Add(entry *RequestLog) {
	if entry.ID == "" {
		entry.ID = generateID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	b.mu.Lock()
	if len(b.logs) >= b.maxCapacity {
		b.logs = b.logs[1:]
	}
	b.logs = append(b.logs, entry)

	// Notify active SSE subscribers
	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
			// Non-blocking drop if channel is full
		}
	}
	b.mu.Unlock()
}

// GetRecent returns a copy of recent request logs (latest first) up to limit
func (b *LogBuffer) GetRecent(limit int) []*RequestLog {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := len(b.logs)
	if limit <= 0 || limit > n {
		limit = n
	}

	result := make([]*RequestLog, limit)
	for i := 0; i < limit; i++ {
		result[i] = b.logs[n-1-i]
	}
	return result
}

// Clear empties the request log buffer
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = make([]*RequestLog, 0, b.maxCapacity)
}

// Subscribe returns a channel that receives newly emitted request logs
func (b *LogBuffer) Subscribe() chan *RequestLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *RequestLog, 100)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a request log subscription channel
func (b *LogBuffer) Unsubscribe(ch chan *RequestLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.subscribers, ch)
	close(ch)
}

// Add appends a new system log to the ring buffer and notifies subscribers
func (b *SystemLogBuffer) Add(entry *SystemLog) {
	if entry.ID == "" {
		entry.ID = generateID()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	b.mu.Lock()
	if len(b.logs) >= b.maxCapacity {
		b.logs = b.logs[1:]
	}
	b.logs = append(b.logs, entry)

	// Notify active SSE subscribers
	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	b.mu.Unlock()
}

// GetRecent returns a copy of recent system logs (latest first) up to limit
func (b *SystemLogBuffer) GetRecent(limit int) []*SystemLog {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := len(b.logs)
	if limit <= 0 || limit > n {
		limit = n
	}

	result := make([]*SystemLog, limit)
	for i := 0; i < limit; i++ {
		result[i] = b.logs[n-1-i]
	}
	return result
}

// Clear empties the system log buffer
func (b *SystemLogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.logs = make([]*SystemLog, 0, b.maxCapacity)
}

// Subscribe returns a channel that receives newly emitted system logs
func (b *SystemLogBuffer) Subscribe() chan *SystemLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *SystemLog, 100)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a system log subscription channel
func (b *SystemLogBuffer) Unsubscribe(ch chan *SystemLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.subscribers, ch)
	close(ch)
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func inferSource(format, msg string) string {
	lower := strings.ToLower(format + " " + msg)
	if strings.Contains(lower, "ddns") || strings.Contains(lower, "cloudflare") || strings.Contains(lower, "dns record") {
		return "DDNS"
	}
	if strings.Contains(lower, "tls") || strings.Contains(lower, "cert") || strings.Contains(lower, "acme") || strings.Contains(lower, "handshake") || strings.Contains(lower, "autocert") {
		return "TLS"
	}
	if strings.Contains(lower, "proxy") || strings.Contains(lower, "target") || strings.Contains(lower, "upstream") || strings.Contains(lower, "route") {
		return "PROXY"
	}
	if strings.Contains(lower, "admin") || strings.Contains(lower, "login") || strings.Contains(lower, "auth") || strings.Contains(lower, "password") || strings.Contains(lower, "jwt") {
		return "AUTH"
	}
	return "SYSTEM"
}

// LogSystem adds an explicit system log and prints to stdout
func LogSystem(level, source, format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	GetSystemLogBuffer().Add(&SystemLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     level,
		Source:    source,
		Message:   msg,
	})
	log.Printf("[%s] [%s] %s", level, source, msg)
}

// SystemInfo logs an info message with an explicit source
func SystemInfo(source, format string, v ...any) {
	LogSystem("INFO", source, format, v...)
}

// SystemWarn logs a warning message with an explicit source
func SystemWarn(source, format string, v ...any) {
	LogSystem("WARN", source, format, v...)
}

// SystemError logs an error message with an explicit source
func SystemError(source, format string, v ...any) {
	LogSystem("ERROR", source, format, v...)
}

// SystemDebug logs a debug message with an explicit source
func SystemDebug(source, format string, v ...any) {
	LogSystem("DEBUG", source, format, v...)
}

// Info logs an informational message to stdout and system log buffer
func Info(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	source := inferSource(format, msg)
	cleanMsg := strings.TrimPrefix(msg, "["+source+"] ")
	GetSystemLogBuffer().Add(&SystemLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     "INFO",
		Source:    source,
		Message:   cleanMsg,
	})
	log.Printf("[INFO] "+format, v...)
}

// Warn logs a warning message to stdout and system log buffer
func Warn(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	source := inferSource(format, msg)
	cleanMsg := strings.TrimPrefix(msg, "["+source+"] ")
	GetSystemLogBuffer().Add(&SystemLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     "WARN",
		Source:    source,
		Message:   cleanMsg,
	})
	log.Printf("[WARN] "+format, v...)
}

// Error logs an error message to stderr and system log buffer
func Error(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	source := inferSource(format, msg)
	cleanMsg := strings.TrimPrefix(msg, "["+source+"] ")
	GetSystemLogBuffer().Add(&SystemLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     "ERROR",
		Source:    source,
		Message:   cleanMsg,
	})
	log.Printf("[ERROR] "+format, v...)
}

// Debug logs a debug message to stdout and system log buffer
func Debug(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	source := inferSource(format, msg)
	cleanMsg := strings.TrimPrefix(msg, "["+source+"] ")
	GetSystemLogBuffer().Add(&SystemLog{
		ID:        generateID(),
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Source:    source,
		Message:   cleanMsg,
	})
	log.Printf("[DEBUG] "+format, v...)
}

// logWriter captures standard library log output from http.Server and records it
type logWriter struct {
	source string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	line := strings.TrimSpace(string(p))
	if line != "" {
		level := "WARN"
		if strings.Contains(strings.ToLower(line), "error") || strings.Contains(strings.ToLower(line), "failed") {
			level = "ERROR"
		}
		GetSystemLogBuffer().Add(&SystemLog{
			ID:        generateID(),
			Timestamp: time.Now(),
			Level:     level,
			Source:    w.source,
			Message:   line,
		})
	}
	return os.Stderr.Write(p)
}

// NewStandardLogger creates a *log.Logger that routes messages to SystemLogBuffer
func NewStandardLogger(source string) *log.Logger {
	return log.New(&logWriter{source: source}, "", log.LstdFlags)
}

// String returns JSON representation of RequestLog
func (l *RequestLog) String() string {
	b, _ := json.Marshal(l)
	return string(b)
}

// String returns JSON representation of SystemLog
func (l *SystemLog) String() string {
	b, _ := json.Marshal(l)
	return string(b)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
}
