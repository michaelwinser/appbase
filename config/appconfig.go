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
	"strings"

	"gopkg.in/yaml.v3"
)

// AppConfig holds the parsed app.yaml.
type AppConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`

	Store StoreConfig `yaml:"store"`
	Auth  AuthConfig  `yaml:"auth"`

	Environments map[string]EnvOverride `yaml:"environments"`

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

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	RedirectURL  string   `yaml:"redirect_url"`
	AllowedUsers []string `yaml:"allowed_users"`
	ExtraScopes  []string `yaml:"extra_scopes"`
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
