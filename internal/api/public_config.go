package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"faka-gateway/internal/config"
)

// handlePublicConfig 公开配置（前端无需登录可读）
// VULN-022：upstream_base 仅对同站 / Referer 白名单内的请求返回；
// 来自外部的请求只返回 site_name，避免泄露上游基础 URL。
func (d *Deps) handlePublicConfig(c *gin.Context) {
	data := gin.H{
		"site_name": "Vaultlane",
	}
	if originTrusted(c, d.Cfg) {
		data["upstream_base"] = d.Cfg.Upstream.BaseURL
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "success",
		"data": data,
	})
}

// originTrusted 判断请求是否来自可信 origin（同站或白名单）。
// 规则：
//  1. 没有 Origin / Referer 的同源直访（curl、服务端）视为可信；
//  2. Origin 命中 cfg.Server.AllowedOrigins 视为可信；
//  3. Referer host 命中 AllowedOrigins 视为可信（浏览器导航场景）；
//  4. 其余视为不可信，隐藏 upstream_base。
func originTrusted(c *gin.Context, cfg *config.Config) bool {
	allowed := cfg.Server.AllowedOrigins
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		host := u.Host
		if host == "" {
			return false
		}
		for _, a := range allowed {
			if strings.EqualFold(a, host) {
				return true
			}
		}
		return false
	}
	// 浏览器无 Origin 但有 Referer（极少数：HTTP/1.0 老客户端）
	if ref := strings.TrimSpace(c.GetHeader("Referer")); ref != "" {
		u, err := url.Parse(ref)
		if err == nil && u.Host != "" {
			for _, a := range allowed {
				if strings.EqualFold(a, u.Host) {
					return true
				}
			}
		}
	}
	// 无 Origin / Referer：同源直访（curl、server-to-server）默认放行
	return true
}
