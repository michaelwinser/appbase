package cli

import (
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/michaelwinser/appbase/auth"
)

func TestHandlerTransport_InjectsIdentity(t *testing.T) {
	var gotUserID, gotEmail string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = auth.UserID(r)
		gotEmail = auth.Email(r)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	transport := &handlerTransport{
		handler: handler,
		userID:  "test-user",
		email:   "test@example.com",
	}

	req, _ := http.NewRequest("GET", "http://local/api/test", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}
	defer resp.Body.Close()

	if gotUserID != "test-user" {
		t.Errorf("userID = %q, want %q", gotUserID, "test-user")
	}
	if gotEmail != "test@example.com" {
		t.Errorf("email = %q, want %q", gotEmail, "test@example.com")
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", string(body), "ok")
	}
}

func TestHandlerTransport_NoIdentity(t *testing.T) {
	var gotUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = auth.UserID(r)
		w.WriteHeader(http.StatusOK)
	})

	transport := &handlerTransport{
		handler: handler,
	}

	req, _ := http.NewRequest("GET", "http://local/api/test", nil)
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}

	if gotUserID != "" {
		t.Errorf("userID = %q, want empty", gotUserID)
	}
}

func TestHandlerTransport_PreservesRequestBody(t *testing.T) {
	var gotBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
	})

	transport := &handlerTransport{
		handler: handler,
		userID:  "user",
		email:   "user@test.com",
	}

	req, _ := http.NewRequest("POST", "http://local/api/todos", nil)
	req.Body = io.NopCloser(readerFromString(`{"title":"test"}`))
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error: %v", err)
	}

	if gotBody != `{"title":"test"}` {
		t.Errorf("body = %q, want %q", gotBody, `{"title":"test"}`)
	}
}

func TestLocalUserID_Default(t *testing.T) {
	os.Unsetenv("DEV_USER_EMAIL")
	got := LocalUserID()
	if got != "dev@localhost" {
		t.Errorf("LocalUserID() = %q, want %q", got, "dev@localhost")
	}
}

func TestLocalUserID_FromEnv(t *testing.T) {
	os.Setenv("DEV_USER_EMAIL", "custom@test.com")
	defer os.Unsetenv("DEV_USER_EMAIL")

	got := LocalUserID()
	if got != "custom@test.com" {
		t.Errorf("LocalUserID() = %q, want %q", got, "custom@test.com")
	}
}

type stringReader struct {
	s string
	i int
}

func (r *stringReader) Read(p []byte) (n int, err error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n = copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

func readerFromString(s string) io.Reader {
	return &stringReader{s: s}
}
