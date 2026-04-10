package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// stateCookieName is the cookie used to store the OAuth state for CSRF validation.
	stateCookieName = "oauth_state"
	// stateMaxAge is how long the state cookie is valid (10 minutes).
	stateMaxAge = 10 * 60
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
	// ClientID is the Google OAuth client ID.
	// Falls back to GOOGLE_CLIENT_ID env var.
	ClientID string

	// ClientSecret is the Google OAuth client secret.
	// Falls back to GOOGLE_CLIENT_SECRET env var.
	ClientSecret string

	// RedirectURL is the OAuth callback URL. Auto-detected from request if empty.
	// Falls back to GOOGLE_REDIRECT_URL env var.
	RedirectURL string

	// Scopes to request. Defaults to openid, email, profile.
	// Apps can add their own (e.g., calendar.readonly).
	ExtraScopes []string

	// AllowedUsers restricts login to these emails. Empty = allow all.
	// Falls back to ALLOWED_USERS env var.
	AllowedUsers []string
}

// NewGoogleAuth creates a Google OAuth handler.
// Config fields fall back to environment variables if not set.
// Returns nil if no client ID is configured (auth disabled).
func NewGoogleAuth(sessions *SessionStore, config GoogleAuthConfig) *GoogleAuth {
	clientID := config.ClientID
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
	}
	clientSecret := config.ClientSecret
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}
	if clientID == "" || clientSecret == "" {
		return nil
	}

	redirectURL := config.RedirectURL
	if redirectURL == "" {
		redirectURL = os.Getenv("GOOGLE_REDIRECT_URL")
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
		redirectURL:  redirectURL,
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
	return fmt.Sprintf("%s://%s/api/auth/callback", scheme, r.Host)
}

// LoginURL returns the Google OAuth login URL with a random state.
// Sets a state cookie on w for CSRF validation in the callback.
func (g *GoogleAuth) LoginURL(w http.ResponseWriter, r *http.Request) string {
	state := uuid.New().String()
	g.setStateCookie(w, r, state)
	return g.loginURL(r, state)
}

// LoginURLWithState returns the Google OAuth login URL with a specific state.
// Used by CLI login to pass a state that links back to the pending login.
// CLI states (prefixed "cli:") are validated via CLILoginStore, not cookies.
func (g *GoogleAuth) LoginURLWithState(r *http.Request, state string) string {
	return g.loginURL(r, state)
}

// loginURL builds the Google OAuth authorization URL.
func (g *GoogleAuth) loginURL(r *http.Request, state string) string {
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

// ValidateState checks the state parameter against the state cookie.
// Returns nil if valid, error if not. Clears the cookie after validation.
// CLI states (prefixed "cli:") are not validated here — they use CLILoginStore.
func (g *GoogleAuth) ValidateState(w http.ResponseWriter, r *http.Request, state string) error {
	// CLI login states are validated by CLILoginStore, not cookies
	if strings.HasPrefix(state, "cli:") {
		return nil
	}

	cookie, err := r.Cookie(stateCookieName)
	if err != nil || cookie.Value == "" {
		return fmt.Errorf("missing OAuth state cookie — possible CSRF or expired login")
	}

	// Clear the state cookie (single-use)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true,
	})

	// Compare HMAC to prevent timing attacks
	expected := g.stateMAC(cookie.Value)
	actual := g.stateMAC(state)
	if !hmac.Equal(expected, actual) {
		return fmt.Errorf("OAuth state mismatch — possible CSRF")
	}

	return nil
}

// setStateCookie stores the OAuth state in a short-lived cookie for CSRF validation.
func (g *GoogleAuth) setStateCookie(w http.ResponseWriter, r *http.Request, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   stateMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

// stateMAC produces a MAC of the state value for constant-time comparison.
func (g *GoogleAuth) stateMAC(state string) []byte {
	mac := hmac.New(sha256.New, []byte(g.clientSecret))
	mac.Write([]byte(state))
	return mac.Sum(nil)
}

// generateStateToken creates a cryptographically random state token.
func generateStateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
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

	// Store OAuth tokens in session for API access
	tokenExpiry := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if err := g.sessions.UpdateTokens(session.ID, tokens.AccessToken, tokens.RefreshToken, tokenExpiry); err != nil {
		log.Printf("auth: failed to store OAuth tokens: %v", err)
	} else {
		session.AccessToken = tokens.AccessToken
		session.RefreshToken = tokens.RefreshToken
		session.TokenExpiry = tokenExpiry
	}

	// Lazy cleanup
	go g.sessions.DeleteExpired()

	return &LoginResult{
		Session:      session,
		Email:        email,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokenExpiry,
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

// ExchangeRefreshToken performs the OAuth refresh_token grant against Google
// and returns the new access token, the effective refresh token, and its expiry.
//
// If Google rotates the refresh token, the rotated value is returned; otherwise
// the input refreshToken is returned unchanged. Callers are responsible for
// persisting the returned refresh token to their own store.
//
// This is the lower-level entry point for callers (e.g. background jobs) that
// hold a refresh token outside the session store and just need a fresh access
// token. For session-backed callers, use RefreshAccessToken.
func (g *GoogleAuth) ExchangeRefreshToken(ctx context.Context, refreshToken string) (access, newRefresh string, expiry time.Time, err error) {
	if refreshToken == "" {
		return "", "", time.Time{}, fmt.Errorf("no refresh token available")
	}

	data := url.Values{}
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", time.Time{}, fmt.Errorf("token refresh failed: %s", string(body))
	}

	var tokens tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return "", "", time.Time{}, err
	}

	// Google sometimes rotates refresh tokens; preserve the input if not.
	newRefresh = refreshToken
	if tokens.RefreshToken != "" {
		newRefresh = tokens.RefreshToken
	}

	expiry = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	return tokens.AccessToken, newRefresh, expiry, nil
}

// RefreshAccessToken uses the session's refresh token to get a new access token
// from Google. Updates both the session store (when session.ID is set) and the
// in-memory session struct. Returns the new access token.
//
// Sessions with an empty ID are treated as transient: the in-memory struct is
// updated but no store write is attempted. This supports callers that construct
// a Session as a parameter bag for the OAuth exchange, though ExchangeRefreshToken
// is the cleaner API for that use case.
func (g *GoogleAuth) RefreshAccessToken(ctx context.Context, session *Session) (string, error) {
	access, newRefresh, expiry, err := g.ExchangeRefreshToken(ctx, session.RefreshToken)
	if err != nil {
		return "", err
	}

	if session.ID != "" {
		if err := g.sessions.UpdateTokens(session.ID, access, newRefresh, expiry); err != nil {
			return "", fmt.Errorf("storing refreshed token: %w", err)
		}
	}

	session.AccessToken = access
	session.RefreshToken = newRefresh
	session.TokenExpiry = expiry

	return access, nil
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
