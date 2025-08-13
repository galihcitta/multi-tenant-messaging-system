package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/galihcitta/multi-tenant-messaging-system/internal/metrics"
)

// PrometheusMiddleware records API request metrics
func PrometheusMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Record metrics after request is processed
		duration := time.Since(start).Seconds()
		statusCode := strconv.Itoa(c.Writer.Status())
		
		metrics.IncrementAPIRequests(c.Request.Method, c.FullPath(), statusCode)
		metrics.RecordAPIRequestDuration(c.Request.Method, c.FullPath(), duration)
	})
}