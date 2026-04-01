// appconfig.go provides app.yaml loading with environment overrides and secret resolution.
//
// app.yaml is the single source of truth for project config:
//
//	name: my-app
//	port: 3000
//	store:
//	  type: sqlite
//	  path: data/app.db
//	auth:
//	  allowed_users: []
//	environments:
//	  local:
//	    url: http://localhost:3000
//	  production:
//	    url: https://my-app.run.app
//	    store:
//	      type: firestore
//	      gcp_project: my-project
//	    auth:
//	      client_id: ${secret:google-client-id}
//	      client_secret: ${secret:google-client-secret}

package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppConfig holds the parsed app.yaml.
type AppConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`

	Store StoreConfig `yaml:"store"`
	Auth  AuthConfig  `yaml:"auth"`
	GCP   GCPConfig   `yaml:"gcp"`

	Environments map[string]EnvOverride  `yaml:"environments"`
	Targets      map[string]TargetConfig `yaml:"targets"`

	// resolved values after merging environment + secrets
	resolved map[string]string
	env      string // active environment name
}

// StoreConfig holds database configuration.
type StoreConfig struct {
	Type       string `yaml:"type"`
	Path       string `yaml:"path"`
	GCPProject string `yaml:"gcp_project"`
}

// GCPConfig holds GCP-specific configuration.
type GCPConfig struct {
	// APIs lists additional GCP APIs the app needs enabled during provisioning.
	// Infrastructure APIs (Cloud Run, Firestore, etc.) are always enabled.
	APIs []string `yaml:"apis"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	ClientID     string            `yaml:"client_id"`
	ClientSecret string            `yaml:"client_secret"`
	RedirectURL  string            `yaml:"redirect_url"`
	AllowedUsers []string          `yaml:"allowed_users"`
	ExtraScopes  []string          `yaml:"extra_scopes"`
	Tokens       map[string]string `yaml:"tokens"`
}

// TargetConfig holds deployment target configuration.
// Targets describe where and how to deploy — GCP project, region, domain,
// secrets, and Cloud Run settings. Separate from environments, which control
// runtime behavior.
type TargetConfig struct {
	Type         string            `yaml:"type"`          // "cloudrun" (default)
	Project      string            `yaml:"project"`       // GCP project ID
	Region       string            `yaml:"region"`        // e.g. "us-central1"
	Domain       string            `yaml:"domain"`        // custom domain (stable URL)
	SupportEmail string            `yaml:"support_email"` // for OAuth consent screen
	Timeout      int               `yaml:"timeout"`       // Cloud Run timeout seconds
	Env          map[string]string `yaml:"env"`           // extra env vars, supports ${secret:name}
	Store        StoreConfig       `yaml:"store"`
	Auth         AuthConfig        `yaml:"auth"`
}

// EnvOverride holds per-environment config overrides.
type EnvOverride struct {
	URL   string      `yaml:"url"`
	Port  int         `yaml:"port"`
	Store StoreConfig `yaml:"store"`
	Auth  AuthConfig  `yaml:"auth"`
}

// LoadAppConfig reads app.yaml, merges the active environment, and resolves secrets.
// The active environment is determined by APP_ENV (default: "local").
// Secret resolution uses the provided SecretResolver (nil to skip).
func LoadAppConfig(path string, secrets SecretResolver) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Determine active environment
	cfg.env = os.Getenv("APP_ENV")
	if cfg.env == "" {
		cfg.env = "local"
	}

	// Apply environment overrides
	if envCfg, ok := cfg.Environments[cfg.env]; ok {
		cfg.applyOverride(envCfg)
	}

	// Apply defaults
	if cfg.Port == 0 {
		cfg.Port = 3000
	}
	if cfg.Store.Type == "" {
		cfg.Store.Type = "sqlite"
	}
	if cfg.Store.Type == "sqlite" && cfg.Store.Path == "" {
		cfg.Store.Path = "data/app.db"
	}

	// Build flat resolved map
	cfg.resolved = cfg.toFlatMap()

	// Resolve ${secret:name} references
	if secrets != nil {
		for k, v := range cfg.resolved {
			if strings.HasPrefix(v, "${secret:") && strings.HasSuffix(v, "}") {
				secretName := v[9 : len(v)-1]
				resolved, err := secrets.Get(cfg.Name, secretName)
				if err != nil {
					return nil, fmt.Errorf("resolving secret %q for %s: %w", secretName, k, err)
				}
				cfg.resolved[k] = resolved
			}
		}
		// Write resolved values back to struct
		cfg.applyResolved()
	}

	// Env vars override everything (highest priority)
	cfg.applyEnvVars()

	return &cfg, nil
}

// Env returns the active environment name.
func (c *AppConfig) Env() string {
	return c.env
}

// URL returns the URL for the active environment.
func (c *AppConfig) URL() string {
	if envCfg, ok := c.Environments[c.env]; ok {
		return envCfg.URL
	}
	return fmt.Sprintf("http://localhost:%d", c.Port)
}

// URLs returns all environment URLs (for OAuth redirect URI generation).
func (c *AppConfig) URLs() []string {
	var urls []string
	for _, env := range c.Environments {
		if env.URL != "" {
			urls = append(urls, env.URL)
		}
	}
	if len(urls) == 0 {
		urls = append(urls, fmt.Sprintf("http://localhost:%d", c.Port))
	}
	return urls
}

// GCPProject returns the GCP project ID.
func (c *AppConfig) GCPProject() string {
	return c.Store.GCPProject
}

// Region returns the deployment region (from env override or default).
func (c *AppConfig) Region() string {
	return c.resolved["region"]
}

// Target returns the named deployment target. If name is empty:
//   - Returns the only target if exactly one exists
//   - Synthesizes a target from environments.production if no targets section exists
//   - Returns an error if multiple targets exist and no name is specified
func (c *AppConfig) Target(name string) (*TargetConfig, error) {
	if len(c.Targets) > 0 {
		if name == "" {
			if len(c.Targets) == 1 {
				for _, t := range c.Targets {
					return &t, nil
				}
			}
			return nil, fmt.Errorf("multiple targets configured — specify one: %s", strings.Join(c.TargetNames(), ", "))
		}
		if t, ok := c.Targets[name]; ok {
			return &t, nil
		}
		return nil, fmt.Errorf("target %q not found (available: %s)", name, strings.Join(c.TargetNames(), ", "))
	}

	// Backward compat: synthesize from environments.production
	t := TargetConfig{
		Type:   "cloudrun",
		Region: "us-central1",
	}
	if prod, ok := c.Environments["production"]; ok {
		t.Project = prod.Store.GCPProject
		t.Auth = prod.Auth
		t.Store = prod.Store
	}
	if t.Project == "" {
		t.Project = c.Store.GCPProject
	}
	if t.Store.Type == "" {
		t.Store.Type = "firestore"
	}
	return &t, nil
}

// TargetNames returns sorted target names.
func (c *AppConfig) TargetNames() []string {
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// applyOverride merges a per-environment config on top of the base.
func (c *AppConfig) applyOverride(env EnvOverride) {
	if env.Port != 0 {
		c.Port = env.Port
	}
	if env.Store.Type != "" {
		c.Store.Type = env.Store.Type
	}
	if env.Store.Path != "" {
		c.Store.Path = env.Store.Path
	}
	if env.Store.GCPProject != "" {
		c.Store.GCPProject = env.Store.GCPProject
	}
	if env.Auth.ClientID != "" {
		c.Auth.ClientID = env.Auth.ClientID
	}
	if env.Auth.ClientSecret != "" {
		c.Auth.ClientSecret = env.Auth.ClientSecret
	}
	if env.Auth.RedirectURL != "" {
		c.Auth.RedirectURL = env.Auth.RedirectURL
	}
	if len(env.Auth.AllowedUsers) > 0 {
		c.Auth.AllowedUsers = env.Auth.AllowedUsers
	}
	if len(env.Auth.ExtraScopes) > 0 {
		c.Auth.ExtraScopes = env.Auth.ExtraScopes
	}
	if len(env.Auth.Tokens) > 0 {
		c.Auth.Tokens = env.Auth.Tokens
	}
}

// toFlatMap converts the config to a flat key-value map.
func (c *AppConfig) toFlatMap() map[string]string {
	m := map[string]string{
		"name":              c.Name,
		"port":              fmt.Sprintf("%d", c.Port),
		"store.type":        c.Store.Type,
		"store.path":        c.Store.Path,
		"store.gcp_project": c.Store.GCPProject,
		"auth.client_id":    c.Auth.ClientID,
		"auth.client_secret": c.Auth.ClientSecret,
		"auth.redirect_url": c.Auth.RedirectURL,
	}
	if len(c.Auth.AllowedUsers) > 0 {
		m["auth.allowed_users"] = strings.Join(c.Auth.AllowedUsers, ",")
	}
	return m
}

// applyResolved writes the flat resolved map back to the struct fields.
func (c *AppConfig) applyResolved() {
	if v, ok := c.resolved["auth.client_id"]; ok && v != "" {
		c.Auth.ClientID = v
	}
	if v, ok := c.resolved["auth.client_secret"]; ok && v != "" {
		c.Auth.ClientSecret = v
	}
}

// applyEnvVars overrides config with environment variables (highest priority).
func (c *AppConfig) applyEnvVars() {
	if v := os.Getenv("PORT"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &c.Port); n == 1 && err == nil {
			// port updated
		}
	}
	if v := os.Getenv("STORE_TYPE"); v != "" {
		c.Store.Type = v
	}
	if v := os.Getenv("SQLITE_DB_PATH"); v != "" {
		c.Store.Path = v
	}
	if v := os.Getenv("GOOGLE_CLOUD_PROJECT"); v != "" {
		c.Store.GCPProject = v
	}
	if v := os.Getenv("GOOGLE_CLIENT_ID"); v != "" {
		c.Auth.ClientID = v
	}
	if v := os.Getenv("GOOGLE_CLIENT_SECRET"); v != "" {
		c.Auth.ClientSecret = v
	}
	if v := os.Getenv("GOOGLE_REDIRECT_URL"); v != "" {
		c.Auth.RedirectURL = v
	}
	if v := os.Getenv("ALLOWED_USERS"); v != "" {
		c.Auth.AllowedUsers = strings.Split(v, ",")
	}
}

// Deprecated: Use appbase.Config fields to pass config explicitly instead.
// SetEnvVars exports config values as environment variables so that
// existing code (db.New, auth.NewGoogleAuth) picks them up.
// Call this after loading config and before initializing the app.
func (c *AppConfig) SetEnvVars() {
	setIfNotEmpty := func(key, val string) {
		if val != "" && os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
	setIfNotEmpty("PORT", fmt.Sprintf("%d", c.Port))
	setIfNotEmpty("STORE_TYPE", c.Store.Type)
	setIfNotEmpty("SQLITE_DB_PATH", c.Store.Path)
	setIfNotEmpty("GOOGLE_CLOUD_PROJECT", c.Store.GCPProject)
	setIfNotEmpty("GOOGLE_CLIENT_ID", c.Auth.ClientID)
	setIfNotEmpty("GOOGLE_CLIENT_SECRET", c.Auth.ClientSecret)
	setIfNotEmpty("GOOGLE_REDIRECT_URL", c.Auth.RedirectURL)
	if len(c.Auth.AllowedUsers) > 0 {
		setIfNotEmpty("ALLOWED_USERS", strings.Join(c.Auth.AllowedUsers, ","))
	}
}
