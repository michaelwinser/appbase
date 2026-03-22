package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/zalando/go-keyring"
)

// SecretResolver resolves secret values by name.
type SecretResolver interface {
	Get(project, name string) (string, error)
	Set(project, name, value string) error
	Delete(project, name string) error
	List(project string) ([]string, error)
}

// ChainResolver tries multiple resolvers in order, returning the first hit.
// The chain order implements the resolution priority:
// OS keychain → Docker secrets → .env file → GCP Secret Manager → env vars
func NewChainResolver(resolvers ...SecretResolver) SecretResolver {
	return &chainResolver{resolvers: resolvers}
}

type chainResolver struct {
	resolvers []SecretResolver
}

func (c *chainResolver) Get(project, name string) (string, error) {
	// Env var override is always highest priority
	envKey := "SECRET_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if val := os.Getenv(envKey); val != "" {
		return val, nil
	}

	for _, r := range c.resolvers {
		val, err := r.Get(project, name)
		if err == nil && val != "" {
			return val, nil
		}
	}
	return "", fmt.Errorf("secret %q not found in any resolver", name)
}

func (c *chainResolver) Set(project, name, value string) error {
	if len(c.resolvers) == 0 {
		return fmt.Errorf("no resolvers configured")
	}
	return c.resolvers[0].Set(project, name, value)
}

func (c *chainResolver) Delete(project, name string) error {
	if len(c.resolvers) == 0 {
		return fmt.Errorf("no resolvers configured")
	}
	return c.resolvers[0].Delete(project, name)
}

func (c *chainResolver) List(project string) ([]string, error) {
	seen := map[string]bool{}
	var all []string
	for _, r := range c.resolvers {
		names, err := r.List(project)
		if err != nil {
			continue
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				all = append(all, n)
			}
		}
	}
	return all, nil
}

// --- OS Keychain ---

const keychainService = "appbase"

// KeychainResolver stores secrets in the OS keychain.
// Uses macOS Keychain, Linux secret-service (GNOME Keyring), or Windows Credential Manager.
type KeychainResolver struct{}

func (k *KeychainResolver) Get(project, name string) (string, error) {
	return keyring.Get(keychainService+"/"+project, name)
}

func (k *KeychainResolver) Set(project, name, value string) error {
	return keyring.Set(keychainService+"/"+project, name, value)
}

func (k *KeychainResolver) Delete(project, name string) error {
	return keyring.Delete(keychainService+"/"+project, name)
}

func (k *KeychainResolver) List(project string) ([]string, error) {
	// go-keyring doesn't support listing; return nil
	return nil, nil
}

// --- Docker Secrets ---

// DockerSecretResolver reads secrets from /run/secrets/<name>.
// This is the standard Docker Compose secrets mechanism.
type DockerSecretResolver struct{}

func (d *DockerSecretResolver) Get(project, name string) (string, error) {
	path := "/run/secrets/" + name
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (d *DockerSecretResolver) Set(project, name, value string) error {
	return fmt.Errorf("docker secrets are read-only at runtime")
}

func (d *DockerSecretResolver) Delete(project, name string) error {
	return fmt.Errorf("docker secrets are read-only at runtime")
}

func (d *DockerSecretResolver) List(project string) ([]string, error) {
	entries, err := os.ReadDir("/run/secrets")
	if err != nil {
		return nil, nil // no secrets directory — not in Docker
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// --- .env File ---

// EnvFileResolver reads secrets from a .env file.
// Looks for lines matching SECRET_NAME=value or name=value.
type EnvFileResolver struct {
	Path string // defaults to ".env"
}

func (e *EnvFileResolver) path() string {
	if e.Path != "" {
		return e.Path
	}
	return ".env"
}

func (e *EnvFileResolver) Get(project, name string) (string, error) {
	entries, err := e.readAll()
	if err != nil {
		return "", err
	}
	// Try exact name first, then env-style key
	if val, ok := entries[name]; ok {
		return val, nil
	}
	envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if val, ok := entries[envKey]; ok {
		return val, nil
	}
	return "", fmt.Errorf("not found in %s", e.path())
}

func (e *EnvFileResolver) Set(project, name, value string) error {
	return fmt.Errorf(".env secrets should be set manually or via ./ab secret set")
}

func (e *EnvFileResolver) Delete(project, name string) error {
	return fmt.Errorf(".env secrets should be removed manually")
}

func (e *EnvFileResolver) List(project string) ([]string, error) {
	entries, err := e.readAll()
	if err != nil {
		return nil, nil
	}
	var names []string
	for k := range entries {
		names = append(names, k)
	}
	return names, nil
}

func (e *EnvFileResolver) readAll() (map[string]string, error) {
	data, err := os.ReadFile(e.path())
	if err != nil {
		return nil, err
	}
	entries := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			entries[key] = val
		}
	}
	return entries, nil
}

// --- GCP Secret Manager ---

// GCPSecretResolver reads secrets from Google Cloud Secret Manager.
type GCPSecretResolver struct {
	ProjectID string
}

func (g *GCPSecretResolver) Get(project, name string) (string, error) {
	gcpProject := g.ProjectID
	if gcpProject == "" {
		gcpProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if gcpProject == "" {
		return "", fmt.Errorf("GCP project not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("creating secret manager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", gcpProject, name),
	}
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", err
	}
	return string(result.Payload.Data), nil
}

func (g *GCPSecretResolver) Set(project, name, value string) error {
	gcpProject := g.ProjectID
	if gcpProject == "" {
		gcpProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if gcpProject == "" {
		return fmt.Errorf("GCP project not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("creating secret manager client: %w", err)
	}
	defer client.Close()

	// Create the secret (ignore error if it already exists)
	_, _ = client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", gcpProject),
		SecretId: name,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})

	// Add the secret version
	_, err = client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s/secrets/%s", gcpProject, name),
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	})
	return err
}

func (g *GCPSecretResolver) Delete(project, name string) error {
	gcpProject := g.ProjectID
	if gcpProject == "" {
		gcpProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", gcpProject, name),
	})
}

func (g *GCPSecretResolver) List(project string) ([]string, error) {
	gcpProject := g.ProjectID
	if gcpProject == "" {
		gcpProject = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if gcpProject == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, nil
	}
	defer client.Close()

	iter := client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", gcpProject),
	})

	var names []string
	for {
		s, err := iter.Next()
		if err != nil {
			break
		}
		// Extract name from "projects/xxx/secrets/name"
		parts := strings.Split(s.Name, "/")
		if len(parts) > 0 {
			names = append(names, parts[len(parts)-1])
		}
	}
	return names, nil
}

// DefaultResolver creates the standard secret resolution chain:
// OS keychain → Docker secrets → .env → GCP Secret Manager → env vars
func DefaultResolver(gcpProject string) SecretResolver {
	return NewChainResolver(
		&KeychainResolver{},
		&DockerSecretResolver{},
		&EnvFileResolver{},
		&GCPSecretResolver{ProjectID: gcpProject},
	)
}
