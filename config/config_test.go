package config

import (
	"os"
	"testing"
)

func TestGet_FromEnv(t *testing.T) {
	os.Setenv("MY_KEY", "from-env")
	defer os.Unsetenv("MY_KEY")

	c := New(map[string]string{"my.key": "from-default"})
	if got := c.Get("my.key"); got != "from-env" {
		t.Fatalf("expected 'from-env', got %q", got)
	}
}

func TestGet_FromDefaults(t *testing.T) {
	c := New(map[string]string{"app.name": "todo"})
	if got := c.Get("app.name"); got != "todo" {
		t.Fatalf("expected 'todo', got %q", got)
	}
}

func TestGet_EnvOverridesDefault(t *testing.T) {
	os.Setenv("APP_NAME", "overridden")
	defer os.Unsetenv("APP_NAME")

	c := New(map[string]string{"app.name": "default"})
	if got := c.Get("app.name"); got != "overridden" {
		t.Fatalf("expected 'overridden', got %q", got)
	}
}

func TestGetOr_Fallback(t *testing.T) {
	c := New(nil)
	if got := c.GetOr("missing", "fallback"); got != "fallback" {
		t.Fatalf("expected 'fallback', got %q", got)
	}
}

func TestGetInt(t *testing.T) {
	c := New(map[string]string{"port": "8080"})
	if got := c.GetInt("port", 3000); got != 8080 {
		t.Fatalf("expected 8080, got %d", got)
	}
}

func TestGetInt_Fallback(t *testing.T) {
	c := New(nil)
	if got := c.GetInt("port", 3000); got != 3000 {
		t.Fatalf("expected 3000, got %d", got)
	}
}

func TestGetBool(t *testing.T) {
	c := New(map[string]string{"debug": "true"})
	if !c.GetBool("debug") {
		t.Fatal("expected true")
	}
}

func TestGetList(t *testing.T) {
	c := New(map[string]string{"allowed.users": "a@b.com, c@d.com"})
	list := c.GetList("allowed.users")
	if len(list) != 2 || list[0] != "a@b.com" || list[1] != "c@d.com" {
		t.Fatalf("unexpected list: %v", list)
	}
}

func TestPort_Default(t *testing.T) {
	c := New(nil)
	if got := c.Port(); got != "3000" {
		t.Fatalf("expected 3000, got %s", got)
	}
}

func TestToEnvKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"store.type", "STORE_TYPE"},
		{"auth.client-id", "AUTH_CLIENT_ID"},
		{"port", "PORT"},
		{"google.cloud.project", "GOOGLE_CLOUD_PROJECT"},
	}
	for _, tt := range tests {
		got := toEnvKey(tt.input)
		if got != tt.expected {
			t.Errorf("toEnvKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
