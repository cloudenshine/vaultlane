package admin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// sessionCookieOptions cookie 选项
func sessionCookieOptions(secure bool) []string {
	parts := []string{
		cookieName + "=",
		"Path=/",
		"HttpOnly",
		"SameSite=Lax",
	}
	if secure {
		parts = append(parts, "Secure")
	}
	return parts
}

// SetSessionCookie 写会话 cookie
func SetSessionCookie(c *gin.Context, value string, maxAgeSec int, secure bool) {
	sameSite := http.SameSiteLaxMode
	c.SetSameSite(sameSite)
	c.SetCookie(cookieName, value, maxAgeSec, "/", "", secure, true)
}

// ClearSessionCookie 清 cookie
func ClearSessionCookie(c *gin.Context, secure bool) {
	c.SetCookie(cookieName, "", -1, "/", "", secure, true)
}

// ReadSessionCookie 读 cookie
func ReadSessionCookie(c *gin.Context) string {
	v, err := c.Cookie(cookieName)
	if err != nil {
		// 兜底：从 Authorization Header 读（CLI 调试用）
		h := c.GetHeader("Authorization")
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimPrefix(h, "Bearer ")
		}
		return ""
	}
	return v
}

// LoginPage 登录页 HTML
const LoginPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <title>码仓 · 管理后台</title>
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='12' fill='%231F3A5F'/%3E%3Ctext x='32' y='44' text-anchor='middle' font-family='serif' font-size='32' fill='%23FAFAF7'%3E码%3C/text%3E%3C/svg%3E" />
  <style>
    *{box-sizing:border-box;margin:0;padding:0}
    body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC",sans-serif;
         background:#FAFAF7;color:#1F2937;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:20px}
    .card{background:#fff;border-radius:12px;box-shadow:0 4px 24px rgba(31,58,95,.08);padding:48px 40px;width:100%;max-width:400px}
    h1{font-size:24px;color:#1F3A5F;margin-bottom:8px;font-weight:600}
    .sub{color:#6B7280;font-size:14px;margin-bottom:32px}
    label{display:block;margin-bottom:6px;font-size:14px;color:#374151}
    input{width:100%;padding:12px 14px;border:1px solid #E5E7EB;border-radius:6px;font-size:14px;
          transition:border-color .2s,box-shadow .2s;background:#FAFAF7}
    input:focus{outline:none;border-color:#1F3A5F;box-shadow:0 0 0 3px rgba(31,58,95,.08);background:#fff}
    .field{margin-bottom:18px}
    button{width:100%;padding:12px;background:#1F3A5F;color:#fff;border:none;border-radius:6px;
           font-size:15px;font-weight:500;cursor:pointer;transition:background .2s}
    button:hover{background:#15294A}
    button:disabled{background:#9CA3AF;cursor:not-allowed}
    .err{color:#B23A48;font-size:13px;margin-top:8px;display:none}
    .err.show{display:block}
  </style>
</head>
<body>
  <div class="card">
    <h1>码仓 · 管理后台</h1>
    <div class="sub">码仓 MASTORE · 内部使用</div>
    <form id="f" autocomplete="off">
      <div class="field">
        <label>用户名</label>
        <input id="u" name="username" type="text" required autofocus />
      </div>
      <div class="field">
        <label>密码</label>
        <input id="p" name="password" type="password" required />
      </div>
      <button id="btn" type="submit">登录</button>
      <div id="err" class="err"></div>
    </form>
  </div>
  <script>
    document.getElementById('f').addEventListener('submit', async (e) => {
      e.preventDefault();
      const errEl = document.getElementById('err');
      const btn = document.getElementById('btn');
      errEl.classList.remove('show');
      btn.disabled = true; btn.textContent = '登录中…';
      try {
        const r = await fetch('/admin/api/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username: u.value, password: p.value }),
          credentials: 'same-origin'
        });
        const j = await r.json();
        if (j.code === 200) { location.href = '/admin/dashboard'; return; }
        errEl.textContent = j.msg || '登录失败';
        errEl.classList.add('show');
      } catch (err) {
        errEl.textContent = '网络错误：' + err.message;
        errEl.classList.add('show');
      } finally {
        btn.disabled = false; btn.textContent = '登录';
      }
    });
  </script>
</body>
</html>
`

// DashboardSPA 仪表盘 SPA（极简版）
const DashboardSPAHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <title>码仓 · 管理后台</title>
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='12' fill='%231F3A5F'/%3E%3Ctext x='32' y='44' text-anchor='middle' font-family='serif' font-size='32' fill='%23FAFAF7'%3E码%3C/text%3E%3C/svg%3E" />
  <link rel="stylesheet" href="/admin/static/style.css" />
</head>
<body>
  <header class="top">
    <div class="brand"><span class="mark">码</span><span>码仓 · 管理后台</span></div>
    <nav class="nav">
      <a href="#dashboard" class="active" data-tab="dashboard">仪表盘</a>
      <a href="#stats" data-tab="stats">财务统计</a>
      <a href="#pool" data-tab="pool">商品池</a>
      <a href="#commodities" data-tab="commodities">自营</a>
      <a href="#orders" data-tab="orders">订单</a>
      <a href="#secrets" data-tab="secrets">卡密</a>
      <a href="#downstream" data-tab="downstream">下游网关</a>
      <a href="#settings" data-tab="settings">设置</a>
    </nav>
    <div class="user">
      <span id="user-name">—</span>
      <button id="logout">退出</button>
    </div>
  </header>
  <main id="main"><div class="loading">加载中…</div></main>
  <script>
    // 服务端注入的模块开关（与 config.yaml 的 modules 段同步）
    window.MODULES = __MODULES_JSON__;
  </script>
  <script src="/admin/static/app.js"></script>
</body>
</html>
`

// renderDashboardSPA 把模块开关注入到 SPA HTML
func renderDashboardSPA(modulesJSON string) string {
	return strings.Replace(DashboardSPAHTML, "__MODULES_JSON__", modulesJSON, 1)
}
