package logger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
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

// LogBuffer maintains an in-memory ring buffer of recent logs and broadcasts to subscribers
type LogBuffer struct {
	mu          sync.RWMutex
	logs        []*RequestLog
	maxCapacity int
	subscribers map[chan *RequestLog]struct{}
}

var (
	defaultBuffer *LogBuffer
	once          sync.Once
)

// GetLogBuffer returns the singleton log buffer instance
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

// GetRecent returns a copy of recent logs (latest first) up to limit
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

// Subscribe returns a channel that receives newly emitted logs
func (b *LogBuffer) Subscribe() chan *RequestLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *RequestLog, 100)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a subscription channel
func (b *LogBuffer) Unsubscribe(ch chan *RequestLog) {
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

// Info logs an informational message to stdout
func Info(format string, v ...any) {
	log.Printf("[INFO] "+format, v...)
}

// Warn logs a warning message to stdout
func Warn(format string, v ...any) {
	log.Printf("[WARN] "+format, v...)
}

// Error logs an error message to stderr
func Error(format string, v ...any) {
	log.Printf("[ERROR] "+format, v...)
}

// Debug logs a debug message
func Debug(format string, v ...any) {
	log.Printf("[DEBUG] "+format, v...)
}

// FormatLog returns JSON string representation of a RequestLog
func (l *RequestLog) String() string {
	b, _ := json.Marshal(l)
	return string(b)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
}
