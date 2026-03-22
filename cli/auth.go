package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keychainService = "appbase"
	sessionKeyName  = "cli-session"
)

// LoginBrowser initiates a browser-based OAuth login against the server.
// Opens the browser, polls for completion, and stores the session in the keychain.
func LoginBrowser(serverURL, appName string) error {
	// Step 1: Request a CLI login from the server
	resp, err := http.Post(serverURL+"/api/auth/cli-login", "application/json", nil)
	if err != nil {
		return fmt.Errorf("contacting server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s", string(body))
	}

	var loginResp struct {
		LoginURL string `json:"loginURL"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("parsing login response: %w", err)
	}

	// Step 2: Open browser
	fmt.Println("Opening browser for login...")
	fmt.Printf("If the browser doesn't open, visit:\n  %s\n\n", loginResp.LoginURL)
	openBrowser(loginResp.LoginURL)

	// Step 3: Poll for completion
	fmt.Print("Waiting for login...")
	sessionID, email, err := pollForSession(serverURL, loginResp.Token, 5*time.Minute)
	if err != nil {
		fmt.Println(" failed.")
		return err
	}
	fmt.Println(" done.")

	// Step 4: Store session in keychain
	if err := keyring.Set(keychainService+"/"+appName, sessionKeyName, sessionID); err != nil {
		return fmt.Errorf("storing session in keychain: %w", err)
	}
	// Also store the server URL so subsequent commands know where to go
	if err := keyring.Set(keychainService+"/"+appName, "cli-server", serverURL); err != nil {
		return fmt.Errorf("storing server URL in keychain: %w", err)
	}

	fmt.Printf("Logged in as %s\n", email)
	return nil
}

// Logout removes the CLI session from the keychain.
func Logout(appName string) error {
	keyring.Delete(keychainService+"/"+appName, sessionKeyName)
	keyring.Delete(keychainService+"/"+appName, "cli-server")
	fmt.Println("Logged out.")
	return nil
}

// Whoami returns the email of the currently logged-in user.
func Whoami(serverURL, appName string) error {
	client, err := AuthenticatedClient(appName)
	if err != nil {
		return fmt.Errorf("not logged in — run: %s login", appName)
	}

	if serverURL == "" {
		serverURL, _ = keyring.Get(keychainService+"/"+appName, "cli-server")
	}
	if serverURL == "" {
		return fmt.Errorf("no server URL — run: %s login --server URL", appName)
	}

	req, _ := http.NewRequest("GET", serverURL+"/api/auth/status", nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("contacting server: %w", err)
	}
	defer resp.Body.Close()

	var status struct {
		LoggedIn bool   `json:"loggedIn"`
		Email    string `json:"email"`
	}
	json.NewDecoder(resp.Body).Decode(&status)

	if !status.LoggedIn {
		return fmt.Errorf("session expired — run: %s login", appName)
	}
	fmt.Printf("Logged in as %s\n  Server: %s\n", status.Email, serverURL)
	return nil
}

// AuthenticatedClient returns an http.Client with the session cookie set.
// Returns an error if not logged in.
func AuthenticatedClient(appName string) (*http.Client, error) {
	sessionID, err := keyring.Get(keychainService+"/"+appName, sessionKeyName)
	if err != nil || sessionID == "" {
		return nil, fmt.Errorf("not logged in")
	}

	jar := &sessionCookieJar{sessionID: sessionID}
	return &http.Client{Jar: jar}, nil
}

// ResolveServerURL determines the server URL from flags, keychain, or default.
func ResolveServerURL(flagValue, appName string) string {
	if flagValue != "" {
		return flagValue
	}
	if saved, err := keyring.Get(keychainService+"/"+appName, "cli-server"); err == nil && saved != "" {
		return saved
	}
	return "http://localhost:3000"
}

// --- internal ---

func pollForSession(serverURL, token string, timeout time.Duration) (sessionID, email string, err error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		fmt.Print(".")

		resp, err := http.Get(fmt.Sprintf("%s/api/auth/cli-poll?token=%s", serverURL, token))
		if err != nil {
			continue
		}

		var result struct {
			Completed bool   `json:"completed"`
			SessionID string `json:"sessionID"`
			Email     string `json:"email"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result.Completed {
			return result.SessionID, result.Email, nil
		}
	}
	return "", "", fmt.Errorf("login timed out after %s", timeout)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}

// sessionCookieJar is a minimal http.CookieJar that adds the session cookie.
type sessionCookieJar struct {
	sessionID string
}

func (j *sessionCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {}
func (j *sessionCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return []*http.Cookie{{Name: "app_session", Value: j.sessionID}}
}
