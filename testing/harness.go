// Package testing provides end-to-end test infrastructure for appbase applications.
//
// The test harness spins up a real HTTP server with an in-memory database,
// runs use case tests against it via HTTP, and reports results keyed by
// use case ID (e.g., UC-1001).
//
// Usage in your app's test file:
//
//	func TestUseCases(t *testing.T) {
//	    h := harness.New(t, setupApp)
//	    h.Run("UC-1001", "Create a todo", func(c *harness.Client) {
//	        resp := c.POST("/api/todos", `{"title":"Buy milk"}`)
//	        c.AssertStatus(resp, 201)
//	        c.AssertJSONHas(resp, "title", "Buy milk")
//	    })
//	}
package testing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SetupFunc initializes the app and returns its HTTP handler.
// Called once before all use case tests.
type SetupFunc func(t *testing.T) http.Handler

// Harness manages an end-to-end test session.
type Harness struct {
	t       *testing.T
	handler http.Handler
	server  *httptest.Server
}

// New creates a test harness. The setupFn initializes the app and returns its handler.
func New(t *testing.T, setupFn SetupFunc) *Harness {
	t.Helper()
	handler := setupFn(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Harness{t: t, handler: handler, server: server}
}

// Run executes a named use case test.
func (h *Harness) Run(ucID, description string, fn func(c *Client)) {
	h.t.Run(fmt.Sprintf("%s_%s", ucID, sanitize(description)), func(t *testing.T) {
		client := &Client{
			t:       t,
			baseURL: h.server.URL,
			http:    h.server.Client(),
			cookies: nil,
		}
		fn(client)
	})
}

// Client provides HTTP methods for use case tests.
type Client struct {
	t       *testing.T
	baseURL string
	http    *http.Client
	cookies []*http.Cookie
	headers map[string]string
}

// SetCookie adds a cookie to the client for subsequent requests.
// Use this with SessionStore.Create() to simulate an authenticated user.
func (c *Client) SetCookie(name, value string) {
	c.cookies = append(c.cookies, &http.Cookie{Name: name, Value: value})
}

// SetHeader adds a header to the client for subsequent requests.
// Use with APPBASE_TEST_MODE=true and X-Test-User for test authentication.
func (c *Client) SetHeader(name, value string) {
	if c.headers == nil {
		c.headers = make(map[string]string)
	}
	c.headers[name] = value
}

// Response wraps an HTTP response with helpers.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	cookies    []*http.Cookie
}

// GET performs a GET request.
func (c *Client) GET(path string) *Response {
	return c.do("GET", path, "")
}

// POST performs a POST request with a JSON body.
func (c *Client) POST(path, body string) *Response {
	return c.do("POST", path, body)
}

// PUT performs a PUT request with a JSON body.
func (c *Client) PUT(path, body string) *Response {
	return c.do("PUT", path, body)
}

// PATCH performs a PATCH request with a JSON body.
func (c *Client) PATCH(path, body string) *Response {
	return c.do("PATCH", path, body)
}

// DELETE performs a DELETE request.
func (c *Client) DELETE(path string) *Response {
	return c.do("DELETE", path, "")
}

func (c *Client) do(method, path, body string) *Response {
	c.t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Add persistent headers (e.g., X-Test-User)
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	// Add cookies from previous responses (session tracking)
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}

	// Save cookies for subsequent requests
	c.cookies = append(c.cookies, resp.Cookies()...)

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
		cookies:    resp.Cookies(),
	}
}

// AssertStatus checks the response status code.
func (c *Client) AssertStatus(r *Response, expected int) {
	c.t.Helper()
	if r.StatusCode != expected {
		c.t.Fatalf("expected status %d, got %d. Body: %s", expected, r.StatusCode, string(r.Body))
	}
}

// AssertJSONHas checks that the response body contains a JSON field with the expected value.
func (c *Client) AssertJSONHas(r *Response, key string, expected interface{}) {
	c.t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(r.Body, &data); err != nil {
		c.t.Fatalf("response is not valid JSON: %s", string(r.Body))
	}
	got, ok := data[key]
	if !ok {
		c.t.Fatalf("JSON field %q not found in response: %s", key, string(r.Body))
	}
	// Compare as strings for simplicity
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", expected) {
		c.t.Fatalf("JSON field %q: expected %v, got %v", key, expected, got)
	}
}

// AssertJSONArray checks that the response body is a JSON array of the expected length.
func (c *Client) AssertJSONArray(r *Response, expectedLen int) {
	c.t.Helper()
	var data []interface{}
	if err := json.Unmarshal(r.Body, &data); err != nil {
		c.t.Fatalf("response is not a JSON array: %s", string(r.Body))
	}
	if len(data) != expectedLen {
		c.t.Fatalf("expected array of length %d, got %d", expectedLen, len(data))
	}
}

// JSON parses the response body as a map.
func (r *Response) JSON() map[string]interface{} {
	var data map[string]interface{}
	json.Unmarshal(r.Body, &data)
	return data
}

// JSONArray parses the response body as a slice.
func (r *Response) JSONArray() []map[string]interface{} {
	var data []map[string]interface{}
	json.Unmarshal(r.Body, &data)
	return data
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	return s
}
