package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimiter returns a Gin middleware that limits requests per client IP using
// a Redis counter. Clients exceeding maxRequests within the window receive HTTP 429.
func RateLimiter(client *redis.Client, maxRequests int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("rate_limit:%s", ip)
		ctx := context.Background()

		// Increment the counter atomically.
		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			// Redis unavailable – fail open so the service keeps running.
			c.Next()
			return
		}

		// Set the expiry only on the first request within the window.
		if count == 1 {
			client.Expire(ctx, key, window)
		}

		remaining := int64(maxRequests) - count
		c.Header("X-RateLimit-Limit", strconv.Itoa(maxRequests))
		c.Header("X-RateLimit-Window", strconv.Itoa(int(window.Seconds()))+"s")

		if count > int64(maxRequests) {
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded – try again later",
			})
			return
		}

		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		c.Next()
	}
}
