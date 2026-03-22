package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create or update app.yaml (interactive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			reader := bufio.NewReader(os.Stdin)

			curName := ""
			curProject := ""
			curRegion := "us-central1"
			curPort := "3000"

			// Read existing values
			if p, err := loadProject(); err == nil {
				curName = p.Name
				curProject = p.GCPProject
				curRegion = p.Region
				if p.Port > 0 {
					curPort = fmt.Sprintf("%d", p.Port)
				}
				fmt.Println("Updating existing config")
			} else {
				fmt.Println("Creating new app config")
			}

			name := prompt(reader, "App name", curName, "my-app")
			project := prompt(reader, "GCP project ID", curProject, "")
			port := prompt(reader, "Port", curPort, "3000")
			region := prompt(reader, "Region", curRegion, "us-central1")

			// Port conflict check
			cwd, _ := os.Getwd()
			parent := filepath.Dir(cwd)
			entries, _ := os.ReadDir(parent)
			for _, e := range entries {
				if !e.IsDir() || filepath.Join(parent, e.Name()) == cwd {
					continue
				}
				siblingYAML := filepath.Join(parent, e.Name(), "app.yaml")
				data, err := os.ReadFile(siblingYAML)
				if err != nil {
					continue
				}
				for _, line := range strings.Split(string(data), "\n") {
					if strings.TrimSpace(line) == "port: "+port {
						fmt.Printf("  WARNING: Port %s is also used by %s\n", port, e.Name())
						confirm := prompt(reader, "  Continue anyway? [y/N]", "", "n")
						if confirm != "y" && confirm != "Y" {
							fmt.Println("Aborted.")
							return nil
						}
					}
				}
			}

			// Write app.yaml
			yaml := fmt.Sprintf(`name: %s
port: %s

store:
  type: sqlite
  path: data/app.db

environments:
  local:
    url: http://localhost:%s

  production:
    url: https://%s.run.app
    store:
      type: firestore
      gcp_project: %s
    auth:
      client_id: ${secret:google-client-id}
      client_secret: ${secret:google-client-secret}
`, name, port, port, name, project)

			if err := os.WriteFile("app.yaml", []byte(yaml), 0644); err != nil {
				return fmt.Errorf("writing app.yaml: %w", err)
			}
			fmt.Println("Wrote app.yaml")

			// Write app.json for backward compat
			jsonContent := fmt.Sprintf(`{
  "name": "%s",
  "gcpProject": "%s",
  "region": "%s",
  "urls": [
    "http://localhost:%s"
  ]
}
`, name, project, region, port)

			if err := os.WriteFile("app.json", []byte(jsonContent), 0644); err != nil {
				return fmt.Errorf("writing app.json: %w", err)
			}
			fmt.Println("Wrote app.json")
			return nil
		},
	}
}

func prompt(reader *bufio.Reader, label, current, fallback string) string {
	def := current
	if def == "" {
		def = fallback
	}
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return def
	}
	return input
}
