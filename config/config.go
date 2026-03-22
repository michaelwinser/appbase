// Package config provides application configuration with a layered resolution strategy.
//
// Today: reads from environment variables.
// Future: config files (YAML), secret managers (GCP Secret Manager, Vault).
//
// The interface is stable — apps call config.Get("key") and the resolution
// strategy can change without modifying app code.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config provides typed access to application configuration.
// Resolution order: environment variables (highest priority), then defaults.
type Config struct {
	defaults map[string]string
}

// New creates a Config with optional default values.
func New(defaults map[string]string) *Config {
	if defaults == nil {
		defaults = make(map[string]string)
	}
	return &Config{defaults: defaults}
}

// Get returns a config value by key. Checks env vars first, then defaults.
// Keys are mapped to env vars by uppercasing and replacing dots/dashes with underscores.
// e.g., "store.type" → STORE_TYPE, "auth.client-id" → AUTH_CLIENT_ID
func (c *Config) Get(key string) string {
	// Check env var
	envKey := toEnvKey(key)
	if val := os.Getenv(envKey); val != "" {
		return val
	}
	// Check defaults
	return c.defaults[key]
}

// GetOr returns a config value, falling back to the provided default.
func (c *Config) GetOr(key, fallback string) string {
	if val := c.Get(key); val != "" {
		return val
	}
	return fallback
}

// GetInt returns an integer config value.
func (c *Config) GetInt(key string, fallback int) int {
	val := c.Get(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}

// GetBool returns a boolean config value.
func (c *Config) GetBool(key string) bool {
	val := strings.ToLower(c.Get(key))
	return val == "true" || val == "1" || val == "yes"
}

// GetList returns a comma-separated config value as a string slice.
func (c *Config) GetList(key string) []string {
	val := c.Get(key)
	if val == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(val, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// Set overrides a config value at runtime (useful for tests).
// Note: Get() checks env vars first, so if an env var is set for this key,
// Set() will not override it. Clear the env var first if needed.
func (c *Config) Set(key, value string) {
	c.defaults[key] = value
}

// Port returns the configured server port.
func (c *Config) Port() string {
	return c.GetOr("port", "3000")
}

// StoreType returns the configured store type.
func (c *Config) StoreType() string {
	return c.GetOr("store.type", "sqlite")
}

// toEnvKey converts a dot-notation key to an environment variable name.
// "store.type" → "STORE_TYPE"
// "auth.client-id" → "AUTH_CLIENT_ID"
func toEnvKey(key string) string {
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, "-", "_")
	return strings.ToUpper(key)
}
