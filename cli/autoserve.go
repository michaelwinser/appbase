package cli

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// AutoServe starts an HTTP handler on a random port and returns the URL.
// The server runs in a goroutine. Call the returned stop function to shut it down.
//
// Usage in an app's setup:
//
//	if !appcli.IsServeCommand && serverURL == "" {
//	    url, stop, err := appcli.AutoServe(app.Server().Router())
//	    if err != nil { return err }
//	    defer stop()
//	    serverURL = url
//	}
func AutoServe(handler http.Handler) (url string, stop func(), err error) {
	// Listen on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("autoserve: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	url = fmt.Sprintf("http://127.0.0.1:%d", port)

	srv := &http.Server{Handler: handler}

	// Start serving in background
	go srv.Serve(listener)

	// Wait for the server to be ready
	if err := waitForReady(url, 5*time.Second); err != nil {
		srv.Close()
		return "", nil, err
	}

	stop = func() {
		srv.Close()
	}

	return url, stop, nil
}

// waitForReady polls the health endpoint until the server responds.
func waitForReady(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("autoserve: server not ready after %s", timeout)
}
