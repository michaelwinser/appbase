package auth

import (
	"html/template"
	"net/http"
)

// LoginPageData contains the data passed to the login page template.
type LoginPageData struct {
	AppName       string
	LoginURL      string
	AuthEnabled   bool
	GoogleEnabled bool
	TokenEnabled  bool
	LoggedIn      bool
	Email         string
}

var loginTemplate = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.AppName}} — Sign In</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,-apple-system,sans-serif;min-height:100vh;display:flex;align-items:center;justify-content:center;background:#f5f5f5;color:#333}
.card{background:#fff;border-radius:12px;padding:2.5rem;max-width:400px;width:90%;text-align:center;box-shadow:0 2px 8px rgba(0,0,0,.08)}
h1{font-size:1.5rem;margin-bottom:.5rem}
p{color:#666;margin-bottom:1.5rem;font-size:.95rem}
.btn{display:inline-flex;align-items:center;gap:.5rem;padding:.75rem 1.5rem;border:1px solid #ddd;border-radius:8px;background:#fff;font-size:1rem;cursor:pointer;text-decoration:none;color:#333;transition:background .15s}
.btn:hover{background:#f0f0f0}
.btn svg{width:20px;height:20px}
.disabled{color:#999;font-size:.9rem}
.divider{display:flex;align-items:center;gap:1rem;margin:1.5rem 0;color:#999;font-size:.85rem}
.divider::before,.divider::after{content:"";flex:1;border-top:1px solid #ddd}
.token-form{display:flex;gap:.5rem}
.token-form input{flex:1;padding:.6rem .8rem;border:1px solid #ddd;border-radius:8px;font-size:.95rem}
.token-form button{padding:.6rem 1rem;border:none;border-radius:8px;background:#333;color:#fff;font-size:.95rem;cursor:pointer}
.token-form button:hover{background:#555}
</style>
</head>
<body>
<div class="card">
<h1>{{.AppName}}</h1>
{{if .AuthEnabled}}
<p>Sign in to continue</p>
{{if .GoogleEnabled}}
<a href="{{.LoginURL}}" class="btn">
<svg viewBox="0 0 24 24"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.27-4.74 3.27-8.1z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>
Sign in with Google
</a>
{{end}}
{{if and .GoogleEnabled .TokenEnabled}}
<div class="divider">or</div>
{{end}}
{{if .TokenEnabled}}
<form class="token-form" method="POST" action="/api/auth/token-login">
<input type="password" name="token" placeholder="Enter token" required>
<button type="submit">Sign in</button>
</form>
{{end}}
{{else}}
<p class="disabled">Authentication is not configured.<br>Set auth tokens or Google OAuth credentials to enable.</p>
{{end}}
</div>
</body>
</html>`))

// ServeLoginPage writes the login page HTML response.
// The Google login button links to /api/auth/login (which sets the state cookie
// and redirects to Google). Token auth uses a form POST to /api/auth/token-login.
func ServeLoginPage(w http.ResponseWriter, r *http.Request, appName string, google *GoogleAuth, tokenAuth *TokenAuth) {
	googleEnabled := google != nil && google.IsConfigured()
	tokenEnabled := tokenAuth != nil && tokenAuth.IsConfigured()
	data := LoginPageData{
		AppName:       appName,
		AuthEnabled:   googleEnabled || tokenEnabled,
		GoogleEnabled: googleEnabled,
		TokenEnabled:  tokenEnabled,
		LoginURL:      "/api/auth/login",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	loginTemplate.Execute(w, data)
}
