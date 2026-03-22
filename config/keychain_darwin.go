package config

import (
	"os/exec"
	"strings"
)

// keychainList returns secret names stored in the macOS Keychain for the given service.
func keychainList(service string) ([]string, error) {
	out, err := exec.Command("security", "dump-keychain").Output()
	if err != nil {
		return nil, nil
	}

	// Split output into blocks separated by "keychain:" lines.
	// Each block represents one keychain entry with acct and svce fields.
	var names []string
	var block []string

	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "keychain:") {
			// Process the previous block
			if name := extractFromBlock(block, service); name != "" {
				names = append(names, name)
			}
			block = nil
			continue
		}
		block = append(block, trimmed)
	}
	// Process last block
	if name := extractFromBlock(block, service); name != "" {
		names = append(names, name)
	}

	return names, nil
}

// extractFromBlock checks if a keychain entry block matches the service
// and returns the account name if so.
func extractFromBlock(block []string, service string) string {
	var acct, svce string
	for _, line := range block {
		if strings.Contains(line, `"acct"`) {
			acct = extractBlob(line)
		}
		if strings.Contains(line, `"svce"`) {
			svce = extractBlob(line)
		}
	}
	if svce == service && acct != "" {
		return acct
	}
	return ""
}

// extractBlob extracts the value from a line like: "acct"<blob>="value"
func extractBlob(line string) string {
	marker := `<blob>="`
	idx := strings.Index(line, marker)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(marker):]
	if end := strings.IndexByte(rest, '"'); end >= 0 {
		return rest[:end]
	}
	return ""
}
