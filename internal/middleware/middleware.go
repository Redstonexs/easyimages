package middleware

import (
	"easyimage/config"
	"easyimage/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CheckLogin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不需要登录检查的情况
		if cfg.MustLogin == 0 {
			c.Next()
			return
		}

		if service.IsLoggedIn(c) {
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{
			"result":  "failed",
			"code":    401,
			"message": "请先登录",
		})
		c.Abort()
	}
}

func RequireAdmin(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.IsAdmin(c) {
			c.Redirect(http.StatusFound, "/admin/index.php")
			c.Abort()
			return
		}
		c.Next()
	}
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		c.Next()
	}
}
