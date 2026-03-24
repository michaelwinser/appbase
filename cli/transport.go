package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/michaelwinser/appbase/auth"
	"github.com/spf13/cobra"
)

// handlerTransport is an http.RoundTripper that calls the handler directly
// via httptest, bypassing TCP. Identity is injected into the request context
// so no sessions, cookies, or DevAuth are needed.
type handlerTransport struct {
	handler http.Handler
	userID  string
	email   string
}

func (t *handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.userID != "" {
		ctx := auth.WithIdentity(req.Context(), t.userID, t.email)
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	t.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// ClientForCommand returns an HTTP client and base URL appropriate for the
// current mode. In local mode, returns an in-process transport that calls
// the handler directly. In remote mode, returns a real HTTP client with
// keychain auth.
//
// This replaces the ResolveServerWithAutoServe + AuthenticatedClient pattern.
func ClientForCommand(cmd *cobra.Command, appName string, handler http.Handler) (
	client *http.Client, baseURL string, cleanup func(), err error,
) {
	// Use IsLocalMode (set by PersistentPreRun based on --server flag)
	// rather than comparing resolved URLs, which breaks when the keychain
	// has a saved server URL or the app uses a non-default port.
	if !IsLocalMode {
		serverFlag, _ := cmd.Flags().GetString("server")
		serverURL := ResolveServerURL(serverFlag, appName)
		httpClient, err := AuthenticatedClient(appName)
		if err != nil {
			return nil, "", nil, fmt.Errorf("not logged in — run: %s login", appName)
		}
		return httpClient, serverURL, func() {}, nil
	}

	// Local mode: in-process transport, no TCP
	email := LocalUserID()
	transport := &handlerTransport{
		handler: handler,
		userID:  email,
		email:   email,
	}
	return &http.Client{Transport: transport}, "http://local", func() {}, nil
}

// LocalUserID returns the local user identity used in local/desktop mode.
// Returns the DEV_USER_EMAIL environment variable, or "dev@localhost" by default.
//
// Use this in Pattern A apps (direct store access) for consistent identity:
//
//	todos.Create(appcli.LocalUserID(), args[0])
func LocalUserID() string {
	if email := os.Getenv("DEV_USER_EMAIL"); email != "" {
		return email
	}
	return "dev@localhost"
}
