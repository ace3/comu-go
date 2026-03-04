package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireSyncSecret returns a middleware that validates the X-Sync-Secret header.
// Returns 401 Unauthorized if the header is missing or does not match the expected secret.
func RequireSyncSecret(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("X-Sync-Secret")
		if header == "" || header != secret {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
