package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/chunked-upload/internal/logger"
	"github.com/yourorg/chunked-upload/internal/models"
	"go.uber.org/zap"
)

// bucket is a simple per-IP token bucket.
type bucket struct {
	tokens   float64
	capacity float64
	rate     float64 // tokens per second
	lastSeen time.Time
	mu       sync.Mutex
}

func (b *bucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.lastSeen = now

	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// ipLimiter manages per-IP buckets and prunes stale entries.
type ipLimiter struct {
	buckets  map[string]*bucket
	mu       sync.Mutex
	capacity float64
	rate     float64
}

func newIPLimiter(ratePerMin, burst float64) *ipLimiter {
	l := &ipLimiter{
		buckets:  make(map[string]*bucket),
		capacity: burst,
		rate:     ratePerMin / 60.0,
	}
	go l.cleanup()
	return l
}

func (l *ipLimiter) get(ip string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{
			tokens:   l.capacity,
			capacity: l.capacity,
			rate:     l.rate,
			lastSeen: time.Now(),
		}
		l.buckets[ip] = b
	}
	return b
}

func (l *ipLimiter) cleanup() {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for range tick.C {
		l.mu.Lock()
		for ip, b := range l.buckets {
			b.mu.Lock()
			if time.Since(b.lastSeen) > 10*time.Minute {
				delete(l.buckets, ip)
			}
			b.mu.Unlock()
		}
		l.mu.Unlock()
	}
}

// ─── Public middleware constructors ──────────────────────────────────────────

// GeneralRateLimit applies a 100 req/min limit to general API endpoints.
func GeneralRateLimit() gin.HandlerFunc {
	limiter := newIPLimiter(100, 20)
	return rateLimitMiddleware(limiter, "general")
}

// ChunkUploadRateLimit applies a 700 req/min limit per IP to the chunk
// upload endpoint to allow fast sequential chunk uploads.
func ChunkUploadRateLimit() gin.HandlerFunc {
	limiter := newIPLimiter(700, 100)
	return rateLimitMiddleware(limiter, "chunk_upload")
}

func rateLimitMiddleware(limiter *ipLimiter, name string) gin.HandlerFunc {
	log := logger.Get()
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.get(ip).allow() {
			log.Warn("rate limit exceeded",
				zap.String("ip", ip),
				zap.String("limiter", name),
				zap.String("path", c.FullPath()),
			)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error:   "rate limit exceeded",
				Details: "too many requests – please slow down",
			})
			return
		}
		c.Next()
	}
}

// RequestLogger logs every inbound request with method, path, status, and latency.
func RequestLogger() gin.HandlerFunc {
	log := logger.Get()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.String("ip", c.ClientIP()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}
