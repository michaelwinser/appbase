//go:build !darwin

package config

// keychainList is not supported on non-macOS platforms.
// Linux secret-service and Windows Credential Manager don't have
// a portable list operation via go-keyring.
func keychainList(service string) ([]string, error) {
	return nil, nil
}
