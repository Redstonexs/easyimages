package middleware

import (
	"easyimage/config"
	"easyimage/internal/service"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type hotlinkDomain struct {
	exact  string
	suffix string
}

type hotlinkCache struct {
	mu      sync.RWMutex
	raw     string
	domains []hotlinkDomain
}

func (hc *hotlinkCache) matches(host, raw string) bool {
	if raw == "" {
		return false
	}
	hc.mu.RLock()
	domains := hc.domains
	hit := hc.raw == raw
	hc.mu.RUnlock()

	if !hit {
		parts := strings.Split(raw, ",")
		parsed := make([]hotlinkDomain, 0, len(parts))
		for _, d := range parts {
			d = strings.TrimSpace(strings.ToLower(d))
			if d != "" {
				parsed = append(parsed, hotlinkDomain{exact: d, suffix: "." + d})
			}
		}
		hc.mu.Lock()
		hc.raw = raw
		hc.domains = parsed
		domains = parsed
		hc.mu.Unlock()
	}

	for _, d := range domains {
		if host == d.exact || strings.HasSuffix(host, d.suffix) {
			return true
		}
	}
	return false
}

// HotlinkProtection 返回防盗链中间件。
// 规则：无 Referer 放行；站点自身域名放行；白名单域名放行（含子域名匹配）；其余返回 403。
func HotlinkProtection(cfg *config.Config) gin.HandlerFunc {
	siteHost := ""
	if u, err := url.Parse(cfg.Domain); err == nil {
		siteHost = strings.ToLower(u.Hostname())
	}
	var allowed hotlinkCache

	return func(c *gin.Context) {
		if cfg.HotlinkProtect == 0 {
			c.Next()
			return
		}

		// A shared cache cannot recheck the caller's Referer before using a hit.
		c.Header("Cache-Control", "private, no-store")
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

		if allowed.matches(host, cfg.HotlinkDomains) {
			c.Next()
			return
		}

		c.AbortWithStatus(http.StatusForbidden)
	}
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

// Workspace users must upload through their quota-checked library.
func LegacyUploadOnly(c *gin.Context) {
	if session, ok := service.CurrentSession(c); ok && !session.IsAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"result": "failed", "code": 403, "message": "请在图片库上传",
		})
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
