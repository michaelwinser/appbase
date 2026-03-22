package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAppConfig_Basic(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
port: 4000
store:
  type: sqlite
  path: test.db
`)
	cfg, err := LoadAppConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "test-app" {
		t.Fatalf("expected name=test-app, got %s", cfg.Name)
	}
	if cfg.Port != 4000 {
		t.Fatalf("expected port=4000, got %d", cfg.Port)
	}
	if cfg.Store.Type != "sqlite" {
		t.Fatalf("expected store.type=sqlite, got %s", cfg.Store.Type)
	}
	if cfg.Store.Path != "test.db" {
		t.Fatalf("expected store.path=test.db, got %s", cfg.Store.Path)
	}
}

func TestLoadAppConfig_Defaults(t *testing.T) {
	path := writeTestYAML(t, `name: minimal`)
	cfg, err := LoadAppConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected default port=3000, got %d", cfg.Port)
	}
	if cfg.Store.Type != "sqlite" {
		t.Fatalf("expected default store=sqlite, got %s", cfg.Store.Type)
	}
	if cfg.Store.Path != "data/app.db" {
		t.Fatalf("expected default path=data/app.db, got %s", cfg.Store.Path)
	}
}

func TestLoadAppConfig_EnvironmentOverride(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
port: 3000
store:
  type: sqlite
environments:
  production:
    port: 8080
    store:
      type: firestore
      gcp_project: prod-project
`)
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	cfg, err := LoadAppConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("expected port=8080, got %d", cfg.Port)
	}
	if cfg.Store.Type != "firestore" {
		t.Fatalf("expected store=firestore, got %s", cfg.Store.Type)
	}
	if cfg.Store.GCPProject != "prod-project" {
		t.Fatalf("expected gcp_project=prod-project, got %s", cfg.Store.GCPProject)
	}
	if cfg.Env() != "production" {
		t.Fatalf("expected env=production, got %s", cfg.Env())
	}
}

func TestLoadAppConfig_EnvVarOverridesFile(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
port: 3000
store:
  type: sqlite
`)
	os.Setenv("PORT", "9999")
	os.Setenv("STORE_TYPE", "firestore")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("STORE_TYPE")

	cfg, err := LoadAppConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9999 {
		t.Fatalf("expected port=9999 from env, got %d", cfg.Port)
	}
	if cfg.Store.Type != "firestore" {
		t.Fatalf("expected store=firestore from env, got %s", cfg.Store.Type)
	}
}

func TestLoadAppConfig_URLs(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
environments:
  local:
    url: http://localhost:3000
  production:
    url: https://test-app.run.app
`)
	urls := func() []string {
		cfg, _ := LoadAppConfig(path, nil)
		return cfg.URLs()
	}()

	if len(urls) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls))
	}
}

func TestLoadAppConfig_SecretResolution(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
auth:
  client_id: ${secret:my-client-id}
  client_secret: ${secret:my-client-secret}
`)

	resolver := &mockResolver{
		secrets: map[string]string{
			"my-client-id":     "resolved-id",
			"my-client-secret": "resolved-secret",
		},
	}

	cfg, err := LoadAppConfig(path, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.ClientID != "resolved-id" {
		t.Fatalf("expected resolved-id, got %s", cfg.Auth.ClientID)
	}
	if cfg.Auth.ClientSecret != "resolved-secret" {
		t.Fatalf("expected resolved-secret, got %s", cfg.Auth.ClientSecret)
	}
}

func TestLoadAppConfig_SetEnvVars(t *testing.T) {
	path := writeTestYAML(t, `
name: test-app
port: 4000
store:
  type: firestore
  gcp_project: my-proj
auth:
  client_id: abc
  client_secret: xyz
`)
	// Clear env vars first
	os.Unsetenv("PORT")
	os.Unsetenv("STORE_TYPE")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GOOGLE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CLIENT_SECRET")

	cfg, err := LoadAppConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetEnvVars()

	if os.Getenv("PORT") != "4000" {
		t.Fatalf("expected PORT=4000, got %s", os.Getenv("PORT"))
	}
	if os.Getenv("STORE_TYPE") != "firestore" {
		t.Fatalf("expected STORE_TYPE=firestore, got %s", os.Getenv("STORE_TYPE"))
	}
	if os.Getenv("GOOGLE_CLOUD_PROJECT") != "my-proj" {
		t.Fatalf("expected GOOGLE_CLOUD_PROJECT=my-proj, got %s", os.Getenv("GOOGLE_CLOUD_PROJECT"))
	}
	if os.Getenv("GOOGLE_CLIENT_ID") != "abc" {
		t.Fatalf("expected GOOGLE_CLIENT_ID=abc, got %s", os.Getenv("GOOGLE_CLIENT_ID"))
	}

	// Cleanup
	os.Unsetenv("PORT")
	os.Unsetenv("STORE_TYPE")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GOOGLE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CLIENT_SECRET")
}

// --- mock resolver ---

type mockResolver struct {
	secrets map[string]string
}

func (m *mockResolver) Get(project, name string) (string, error) {
	if v, ok := m.secrets[name]; ok {
		return v, nil
	}
	return "", nil
}

func (m *mockResolver) Set(project, name, value string) error { return nil }
func (m *mockResolver) Delete(project, name string) error     { return nil }
func (m *mockResolver) List(project string) ([]string, error) { return nil, nil }
