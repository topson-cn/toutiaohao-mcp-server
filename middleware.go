package main

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// corsMiddleware CORS 中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowedOrigin := os.Getenv("TOUTIAO_ALLOWED_ORIGIN")
		if allowedOrigin == "" {
			allowedOrigin = "http://127.0.0.1"
		}
		if origin != "" && origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		c.Header("Access-Control-Expose-Headers", "Mcp-Session-Id, Mcp-Protocol-Version")

		if c.Request.Method == "OPTIONS" {
			if origin != "" && origin != allowedOrigin {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func writeAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := os.Getenv("TOUTIAO_HTTP_TOKEN")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, ErrorResponse{
				Success: false,
				Message: "HTTP write API is disabled",
			})
			return
		}

		expected := []byte("Bearer " + token)
		actual := []byte(c.GetHeader("Authorization"))
		if subtle.ConstantTimeCompare(actual, expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Success: false,
				Message: "Unauthorized",
			})
			return
		}

		c.Next()
	}
}

// errorHandlingMiddleware panic 恢复中间件
func errorHandlingMiddleware() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(gin.DefaultWriter, func(c *gin.Context, err interface{}) {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Success: false,
			Message: "Internal server error",
		})
	})
}
