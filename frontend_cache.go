package main

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func frontendCacheHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/public/dist/assets/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		} else if path == "/sw.js" || strings.HasPrefix(path, "/public/dist/") {
			c.Header("Cache-Control", "no-cache")
		}
		if path == "/public/dist/sw.js" {
			c.Header("Service-Worker-Allowed", "/")
		}
		c.Next()
	}
}
