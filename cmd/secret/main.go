// Command secret manages secrets in the OS keychain.
//
// Usage:
//
//	go run ./cmd/secret set <project> <name> <value>
//	go run ./cmd/secret get <project> <name>
//	go run ./cmd/secret delete <project> <name>
//	go run ./cmd/secret list <project>
//	go run ./cmd/secret push <project>   # push keychain secrets to GCP Secret Manager
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/michaelwinser/appbase/config"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	project := os.Args[2]

	keychain := &config.KeychainResolver{}
	gcp := &config.GCPSecretResolver{}

	switch cmd {
	case "set":
		if len(os.Args) < 5 {
			fmt.Fprintln(os.Stderr, "Usage: secret set <project> <name> <value>")
			os.Exit(1)
		}
		name := os.Args[3]
		value := os.Args[4]
		if err := keychain.Set(project, name, value); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Stored %s in keychain for %s\n", name, project)

	case "get":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: secret get <project> <name>")
			os.Exit(1)
		}
		name := os.Args[3]
		val, err := keychain.Get(project, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(val)

	case "delete":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: secret delete <project> <name>")
			os.Exit(1)
		}
		name := os.Args[3]
		if err := keychain.Delete(project, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted %s from keychain\n", name)

	case "list":
		// List from keychain
		keychainNames, _ := keychain.List(project)
		if len(keychainNames) > 0 {
			fmt.Println("Keychain:")
			for _, n := range keychainNames {
				fmt.Printf("  %s\n", n)
			}
		}
		// List from .env
		envResolver := &config.EnvFileResolver{}
		envNames, _ := envResolver.List(project)
		if len(envNames) > 0 {
			fmt.Println(".env:")
			for _, n := range envNames {
				fmt.Printf("  %s\n", n)
			}
		}
		// List from GCP if available
		gcpNames, err := gcp.List(project)
		if err == nil && len(gcpNames) > 0 {
			fmt.Println("GCP Secret Manager:")
			for _, n := range gcpNames {
				fmt.Printf("  %s\n", n)
			}
		}
		if len(keychainNames) == 0 && len(envNames) == 0 && len(gcpNames) == 0 {
			fmt.Println("No secrets found.")
		}

	case "env":
		// Output export statements for known secret names.
		// Used by shell scripts: eval "$(go run ./cmd/secret env project)"
		secretNames := []struct{ keychain, envVar string }{
			{"google-client-id", "GOOGLE_CLIENT_ID"},
			{"google-client-secret", "GOOGLE_CLIENT_SECRET"},
			{"google-redirect-url", "GOOGLE_REDIRECT_URL"},
		}
		for _, s := range secretNames {
			val, err := keychain.Get(project, s.keychain)
			if err == nil && val != "" {
				fmt.Printf("export %s='%s'\n", s.envVar, val)
			}
		}

	case "push":
		// Push keychain secrets to GCP Secret Manager
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: secret push <project> <name1,name2,...>")
			os.Exit(1)
		}
		names := strings.Split(os.Args[3], ",")
		for _, name := range names {
			name = strings.TrimSpace(name)
			val, err := keychain.Get(project, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Skipping %s: not in keychain (%v)\n", name, err)
				continue
			}
			if err := gcp.Set(project, name, val); err != nil {
				fmt.Fprintf(os.Stderr, "Error pushing %s to GCP: %v\n", name, err)
				continue
			}
			fmt.Printf("Pushed %s to GCP Secret Manager\n", name)
		}

	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: secret <command> <project> [args...]

Commands:
  set <project> <name> <value>    Store a secret in the OS keychain
  get <project> <name>            Retrieve a secret from the keychain
  delete <project> <name>         Remove a secret from the keychain
  list <project>                  List known secrets (.env and GCP)
  env <project>                   Output export statements for shell eval
  push <project> <name1,name2>    Push keychain secrets to GCP Secret Manager`)
}
