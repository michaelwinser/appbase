package main

import (
	"fmt"
	"os"

	"github.com/michaelwinser/appbase/config"
	"github.com/spf13/cobra"
)

func targetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target",
		Short: "Manage deployment targets",
	}
	cmd.AddCommand(targetListCmd())
	cmd.AddCommand(targetGetCmd())
	return cmd
}

func targetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured deployment targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAppConfig()
			if err != nil {
				return err
			}

			if len(cfg.Targets) == 0 {
				fmt.Println("No explicit targets configured.")
				fmt.Println("Deploy uses settings from environments.production (backward compat).")
				return nil
			}

			for _, name := range cfg.TargetNames() {
				t := cfg.Targets[name]
				typ := t.Type
				if typ == "" {
					typ = "cloudrun"
				}
				domain := ""
				if t.Domain != "" {
					domain = "  " + t.Domain
				}
				fmt.Printf("%-20s %-10s %-25s %-15s%s\n", name, typ, t.Project, t.Region, domain)
			}
			return nil
		},
	}
}

func targetGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <target-name>",
		Short: "Get target config value (for use in scripts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAppConfig()
			if err != nil {
				return err
			}

			target, err := cfg.Target(args[0])
			if err != nil {
				return err
			}

			field, _ := cmd.Flags().GetString("field")
			if field == "" {
				// Print all fields
				fmt.Printf("type: %s\n", nonEmpty(target.Type, "cloudrun"))
				fmt.Printf("project: %s\n", target.Project)
				fmt.Printf("region: %s\n", target.Region)
				if target.Domain != "" {
					fmt.Printf("domain: %s\n", target.Domain)
				}
				if target.SupportEmail != "" {
					fmt.Printf("support_email: %s\n", target.SupportEmail)
				}
				if target.Timeout > 0 {
					fmt.Printf("timeout: %d\n", target.Timeout)
				}
				if target.Store.Type != "" {
					fmt.Printf("store.type: %s\n", target.Store.Type)
				}
				for k, v := range target.Env {
					fmt.Printf("env.%s: %s\n", k, v)
				}
				return nil
			}

			// Print single field
			val := getTargetField(target, field)
			if val == "" {
				return fmt.Errorf("field %q is empty or not found", field)
			}
			fmt.Print(val)
			return nil
		},
	}
	cmd.Flags().String("field", "", "Get a specific field (type, project, region, domain, support_email, timeout, store.type)")
	return cmd
}

func getTargetField(t *config.TargetConfig, field string) string {
	switch field {
	case "type":
		return nonEmpty(t.Type, "cloudrun")
	case "project":
		return t.Project
	case "region":
		return t.Region
	case "domain":
		return t.Domain
	case "support_email":
		return t.SupportEmail
	case "timeout":
		if t.Timeout > 0 {
			return fmt.Sprintf("%d", t.Timeout)
		}
		return ""
	case "store.type":
		return t.Store.Type
	default:
		// Check env vars
		if len(field) > 4 && field[:4] == "env." {
			return t.Env[field[4:]]
		}
		return ""
	}
}

func nonEmpty(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}

// loadAppConfig loads app.yaml without secret resolution (for CLI commands
// that just need to read config, not resolve secrets).
func loadAppConfig() (*config.AppConfig, error) {
	if _, err := os.Stat("app.yaml"); err != nil {
		return nil, fmt.Errorf("no app.yaml found in current directory")
	}
	return config.LoadAppConfig("app.yaml", nil)
}
