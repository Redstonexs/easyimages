package middleware

import (
	"easyimage/config"
	"easyimage/internal/service"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// HotlinkProtection 返回防盗链中间件。
// 规则：无 Referer 放行；站点自身域名放行；白名单域名放行（含子域名匹配）；其余返回 403。
func HotlinkProtection(cfg *config.Config) gin.HandlerFunc {
	siteHost := ""
	if u, err := url.Parse(cfg.Domain); err == nil {
		siteHost = strings.ToLower(u.Hostname())
	}

	return func(c *gin.Context) {
		if cfg.HotlinkProtect == 0 {
			c.Next()
			return
		}

		referer := c.GetHeader("Referer")
		if referer == "" {
			c.Next()
			return
		}

		u, err := url.Parse(referer)
		if err != nil {
			c.Next()
			return
		}
		host := strings.ToLower(u.Hostname())

		if siteHost != "" && host == siteHost {
			c.Next()
			return
		}

		if isDomainAllowed(host, cfg.HotlinkDomains) {
			c.Next()
			return
		}

		c.AbortWithStatus(http.StatusForbidden)
	}
}

// isDomainAllowed 检查 host 是否匹配白名单中的域名。
// 匹配规则：完全匹配，或 host 是白名单域名的子域名（如 www.example.com 匹配 example.com）。
func isDomainAllowed(host, domains string) bool {
	if domains == "" {
		return false
	}
	for _, d := range strings.Split(domains, ",") {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		if host == d {
			return true
		}
		if strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

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
			c.Redirect(http.StatusFound, "/admin/index")
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
