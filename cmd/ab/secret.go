package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/michaelwinser/appbase/config"
	"github.com/spf13/cobra"
)

func secretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage secrets in the OS keychain",
	}
	cmd.AddCommand(secretSetCmd())
	cmd.AddCommand(secretGetCmd())
	cmd.AddCommand(secretDeleteCmd())
	cmd.AddCommand(secretListCmd())
	cmd.AddCommand(secretImportCmd())
	cmd.AddCommand(secretEnvCmd())
	cmd.AddCommand(secretPushCmd())
	return cmd
}

func projectName() string {
	p, err := loadProject()
	if err != nil {
		return "unknown"
	}
	return p.Name
}

func secretSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <value>",
		Short: "Store a secret in the OS keychain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			k := &config.KeychainResolver{}
			if err := k.Set(projectName(), args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Stored %s in keychain for %s\n", args[0], projectName())
			return nil
		},
	}
}

func secretGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Retrieve a secret from the keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k := &config.KeychainResolver{}
			val, err := k.Get(projectName(), args[0])
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		},
	}
}

func secretDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Remove a secret from the keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k := &config.KeychainResolver{}
			if err := k.Delete(projectName(), args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted %s from keychain\n", args[0])
			return nil
		},
	}
}

func secretListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known secrets from all sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			proj := projectName()
			k := &config.KeychainResolver{}
			found := false

			keychainNames, _ := k.List(proj)
			if len(keychainNames) > 0 {
				fmt.Println("Keychain:")
				for _, n := range keychainNames {
					fmt.Printf("  %s\n", n)
				}
				found = true
			}

			envResolver := &config.EnvFileResolver{}
			envNames, _ := envResolver.List(proj)
			if len(envNames) > 0 {
				fmt.Println(".env:")
				for _, n := range envNames {
					fmt.Printf("  %s\n", n)
				}
				found = true
			}

			gcp := &config.GCPSecretResolver{}
			gcpNames, err := gcp.List(proj)
			if err == nil && len(gcpNames) > 0 {
				fmt.Println("GCP Secret Manager:")
				for _, n := range gcpNames {
					fmt.Printf("  %s\n", n)
				}
				found = true
			}

			if !found {
				fmt.Println("No secrets found.")
			}
			return nil
		},
	}
}

func secretImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <credentials.json>",
		Short: "Import Google OAuth credentials JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("reading %s: %w", args[0], err)
			}

			var creds struct {
				Web struct {
					ClientID     string `json:"client_id"`
					ClientSecret string `json:"client_secret"`
				} `json:"web"`
				Installed struct {
					ClientID     string `json:"client_id"`
					ClientSecret string `json:"client_secret"`
				} `json:"installed"`
			}
			if err := json.Unmarshal(data, &creds); err != nil {
				return fmt.Errorf("parsing JSON: %w", err)
			}

			clientID := creds.Web.ClientID
			clientSecret := creds.Web.ClientSecret
			if clientID == "" {
				clientID = creds.Installed.ClientID
				clientSecret = creds.Installed.ClientSecret
			}
			if clientID == "" || clientSecret == "" {
				return fmt.Errorf("could not find client_id/client_secret in JSON")
			}

			proj := projectName()
			k := &config.KeychainResolver{}
			if err := k.Set(proj, "google-client-id", clientID); err != nil {
				return fmt.Errorf("storing client ID: %w", err)
			}
			if err := k.Set(proj, "google-client-secret", clientSecret); err != nil {
				return fmt.Errorf("storing client secret: %w", err)
			}

			fmt.Printf("Imported OAuth credentials for %s\n", proj)
			fmt.Printf("  google-client-id: %s\n", clientID)
			fmt.Printf("  google-client-secret: (stored in keychain)\n")
			fmt.Printf("\nYou can now delete %s\n", args[0])
			return nil
		},
	}
}

func secretEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "env",
		Short:  "Output export statements for shell eval",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			proj := projectName()
			k := &config.KeychainResolver{}
			secrets := []struct{ keychain, envVar string }{
				{"google-client-id", "GOOGLE_CLIENT_ID"},
				{"google-client-secret", "GOOGLE_CLIENT_SECRET"},
				{"google-redirect-url", "GOOGLE_REDIRECT_URL"},
			}
			for _, s := range secrets {
				val, err := k.Get(proj, s.keychain)
				if err == nil && val != "" {
					fmt.Printf("export %s='%s'\n", s.envVar, val)
				}
			}
			return nil
		},
	}
}

func secretPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <name1,name2,...>",
		Short: "Push keychain secrets to GCP Secret Manager",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			proj := projectName()
			k := &config.KeychainResolver{}
			gcp := &config.GCPSecretResolver{}

			names := strings.Split(args[0], ",")
			for _, name := range names {
				name = strings.TrimSpace(name)
				val, err := k.Get(proj, name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Skipping %s: not in keychain (%v)\n", name, err)
					continue
				}
				if err := gcp.Set(proj, name, val); err != nil {
					fmt.Fprintf(os.Stderr, "Error pushing %s to GCP: %v\n", name, err)
					continue
				}
				fmt.Printf("Pushed %s to GCP Secret Manager\n", name)
			}
			return nil
		},
	}
}
