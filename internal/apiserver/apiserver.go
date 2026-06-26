package apiserver

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/orchestrator"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"golang.org/x/time/rate"
)

type apiServer struct {
	orchestrator orchestrator.Orchestrator
	limiter      *rate.Limiter
	ipLimiter    map[string]*rate.Limiter
}

func NewAPIServer(or orchestrator.Orchestrator) *apiServer {
	return &apiServer{
		orchestrator: or,
		limiter:      rate.NewLimiter(rate.Every(time.Second), 100), // Global rate limit
		ipLimiter:    make(map[string]*rate.Limiter),
	}
}

func (s *apiServer) rateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client IP
		clientIP := c.ClientIP()

		// Check global rate limit
		if !s.limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Global rate limit exceeded",
			})
			c.Abort()
			return
		}

		// Check IP-specific rate limit
		s.mu.Lock()
		ipLimiter, exists := s.ipLimiter[clientIP]
		if !exists {
			ipLimiter = rate.NewLimiter(rate.Every(time.Second), 10) // 10 requests per second per IP
			s.ipLimiter[clientIP] = ipLimiter
		}
		s.mu.Unlock()

		if !ipLimiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "IP rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (s *apiServer) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Security headers
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' https:;")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")
		
		// Prevent MIME type sniffing
		c.Header("X-Content-Type-Options", "nosniff")
		
		// Enable XSS protection
		c.Header("X-XSS-Protection", "1; mode=block")
		
		c.Next()
	}
}

func (s *apiServer) Start() error {
	router := gin.Default()
	
	// Add security headers middleware
	router.Use(s.securityHeaders())

	// Add rate limiting
	router.Use(s.rateLimit())
	
	// ... rest of existing code ...
} 