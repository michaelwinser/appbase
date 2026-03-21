package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GoogleAuth handles Google OAuth 2.0 authentication.
type GoogleAuth struct {
	clientID     string
	clientSecret string
	redirectURL  string // empty = auto-detect from request
	sessions     *SessionStore
	httpClient   *http.Client
	allowedUsers []string
	scopes       []string
}

// GoogleAuthConfig configures Google OAuth.
type GoogleAuthConfig struct {
	// Scopes to request. Defaults to openid, email, profile.
	// Apps can add their own (e.g., calendar.readonly).
	ExtraScopes []string

	// AllowedUsers restricts login to these emails. Empty = allow all.
	AllowedUsers []string
}

// NewGoogleAuth creates a Google OAuth handler from environment variables.
// Returns nil if GOOGLE_CLIENT_ID is not set (auth disabled).
func NewGoogleAuth(sessions *SessionStore, config GoogleAuthConfig) *GoogleAuth {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return nil
	}

	scopes := []string{"openid", "email", "profile"}
	scopes = append(scopes, config.ExtraScopes...)

	// Parse allowed users from env if not provided in config
	allowedUsers := config.AllowedUsers
	if len(allowedUsers) == 0 {
		if val := os.Getenv("ALLOWED_USERS"); val != "" {
			for _, u := range strings.Split(val, ",") {
				u = strings.TrimSpace(u)
				if u != "" {
					allowedUsers = append(allowedUsers, u)
				}
			}
		}
	}

	return &GoogleAuth{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		sessions:     sessions,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		allowedUsers: allowedUsers,
		scopes:       scopes,
	}
}

// IsConfigured returns true if Google OAuth is properly configured.
func (g *GoogleAuth) IsConfigured() bool {
	return g != nil && g.clientID != "" && g.clientSecret != ""
}

// GetRedirectURL returns the OAuth redirect URL, auto-detecting from the request if not configured.
func (g *GoogleAuth) GetRedirectURL(r *http.Request) string {
	if g.redirectURL != "" {
		return g.redirectURL
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/oauth/google/callback", scheme, r.Host)
}

// LoginURL returns the Google OAuth login URL.
func (g *GoogleAuth) LoginURL(r *http.Request) string {
	state := uuid.New().String()
	params := url.Values{}
	params.Set("client_id", g.clientID)
	params.Set("redirect_uri", g.GetRedirectURL(r))
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(g.scopes, " "))
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("state", state)
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

// LoginResult contains the result of a successful login.
type LoginResult struct {
	Session      *Session
	Email        string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       string
}

// HandleCallback exchanges the auth code for tokens, validates the user, and creates a session.
func (g *GoogleAuth) HandleCallback(r *http.Request, code string) (*LoginResult, error) {
	// Exchange code for tokens
	tokens, err := g.exchangeCode(r, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}

	// Get user info
	email, err := g.getUserEmail(r, tokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("getting user email: %w", err)
	}
	if email == "" {
		return nil, fmt.Errorf("could not determine user email")
	}

	// Check allowlist
	if len(g.allowedUsers) > 0 {
		allowed := false
		for _, u := range g.allowedUsers {
			if strings.EqualFold(u, email) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("user %s is not authorized", email)
		}
	}

	// Create session
	session, err := g.sessions.Create(email, email, 30*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	// Lazy cleanup
	go g.sessions.DeleteExpired()

	return &LoginResult{
		Session:      session,
		Email:        email,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second),
		Scopes:       tokens.Scope,
	}, nil
}

// SetSessionCookie sets the session cookie on the response.
func (g *GoogleAuth) SetSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// ClearSessionCookie clears the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true,
	})
}

// --- internal ---

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (g *GoogleAuth) exchangeCode(r *http.Request, code string) (*tokenResponse, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("redirect_uri", g.GetRedirectURL(r))
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(r.Context(), "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokens tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}
	return &tokens, nil
}

func (g *GoogleAuth) getUserEmail(r *http.Request, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get user info")
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", err
	}
	return userInfo.Email, nil
}
