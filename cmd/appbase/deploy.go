package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/michaelwinser/appbase/config"
	"github.com/spf13/cobra"
)

func deployCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Deploy to Cloud Run",
		RunE: func(cmd *cobra.Command, args []string) error {
			p := mustLoadProject()

			if p.GCPProject == "" {
				return fmt.Errorf("gcpProject not set — run 'ab init' or set it in app.yaml")
			}

			storeType := os.Getenv("STORE_TYPE")
			if storeType == "" {
				storeType = "firestore"
			}

			fmt.Printf("Deploying %s to Cloud Run (project: %s, region: %s, store: %s)...\n",
				p.Name, p.GCPProject, p.Region, storeType)

			if storeType == "sqlite" {
				fmt.Println("\n  WARNING: Using SQLite on Cloud Run. Data will be lost on cold starts.")
				fmt.Println("  Use STORE_TYPE=firestore (default) for production.")
			fmt.Println()
			}

			// Load secrets from keychain
			k := &config.KeychainResolver{}
			clientID, _ := k.Get(p.Name, "google-client-id")
			clientSecret, _ := k.Get(p.Name, "google-client-secret")

			// Push secrets to GCP Secret Manager
			envVars := fmt.Sprintf("STORE_TYPE=%s,GOOGLE_CLOUD_PROJECT=%s", storeType, p.GCPProject)
			if v := os.Getenv("ALLOWED_USERS"); v != "" {
				envVars += ",ALLOWED_USERS=" + v
			}

			var secretsFlag string
			if clientID != "" {
				fmt.Println("  Pushing google-client-id to Secret Manager...")
				pushSecret(p.GCPProject, "google-client-id", clientID)
				secretsFlag = "GOOGLE_CLIENT_ID=google-client-id:latest"
			}
			if clientSecret != "" {
				fmt.Println("  Pushing google-client-secret to Secret Manager...")
				pushSecret(p.GCPProject, "google-client-secret", clientSecret)
				if secretsFlag != "" {
					secretsFlag += ","
				}
				secretsFlag += "GOOGLE_CLIENT_SECRET=google-client-secret:latest"
			}

			if secretsFlag != "" {
				fmt.Println("  Secrets will be mounted from Secret Manager (not as env vars).")
			}

			// Build gcloud command
			gcloudArgs := []string{
				"run", "deploy", p.Name,
				"--source", ".",
				"--project=" + p.GCPProject,
				"--region=" + p.Region,
				"--allow-unauthenticated",
				"--clear-base-image",
				"--set-env-vars=" + envVars,
			}
			if secretsFlag != "" {
				gcloudArgs = append(gcloudArgs, "--set-secrets="+secretsFlag)
			}

			if err := run("gcloud", gcloudArgs...); err != nil {
				return fmt.Errorf("deploy failed: %w", err)
			}

			// Capture service URL
			out, err := exec.Command("gcloud", "run", "services", "describe", p.Name,
				"--project="+p.GCPProject,
				"--region="+p.Region,
				"--format=value(status.url)").Output()
			if err == nil {
				serviceURL := strings.TrimSpace(string(out))
				if serviceURL != "" {
					fmt.Printf("\nService URL: %s\n", serviceURL)
				}
			}

			return nil
		},
	}
}

func pushSecret(gcpProject, name, value string) {
	// Check if secret exists
	err := exec.Command("gcloud", "secrets", "describe", name,
		"--project="+gcpProject).Run()
	if err == nil {
		// Add new version
		cmd := exec.Command("gcloud", "secrets", "versions", "add", name,
			"--project="+gcpProject, "--data-file=-")
		cmd.Stdin = strings.NewReader(value)
		cmd.Run()
	} else {
		// Create
		cmd := exec.Command("gcloud", "secrets", "create", name,
			"--project="+gcpProject, "--replication-policy=automatic", "--data-file=-")
		cmd.Stdin = strings.NewReader(value)
		cmd.Run()
	}
}

func provisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision <support-email>",
		Short: "Full GCP setup (project, billing, APIs, resources, OAuth)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := mustLoadProject()
			email := args[0]

			if p.GCPProject == "" {
				return fmt.Errorf("gcpProject not set — run 'ab init' first")
			}

			fmt.Println("================================================")
			fmt.Printf("Provisioning GCP for %s\n", p.Name)
			fmt.Printf("  Project: %s\n", p.GCPProject)
			fmt.Printf("  Contact: %s\n", email)
			fmt.Println("================================================")
			fmt.Println()

			// 1. Project
			fmt.Println("[1/5] Project")
			run("gcloud", "projects", "describe", p.GCPProject)
			run("gcloud", "config", "set", "project", p.GCPProject)

			// 2. Billing
			fmt.Println("\n[2/5] Billing")
			out, _ := exec.Command("gcloud", "billing", "projects", "describe", p.GCPProject,
				"--format=value(billingAccountName)").Output()
			if strings.TrimSpace(string(out)) != "" {
				fmt.Printf("  Billing already linked: %s\n", strings.TrimSpace(string(out)))
			} else {
				fmt.Println("  WARNING: No billing linked. Link manually or pass a billing account.")
			}

			// 3. APIs
			fmt.Println("\n[3/5] APIs")
			apis := []string{
				"cloudbuild.googleapis.com",
				"run.googleapis.com",
				"firestore.googleapis.com",
				"artifactregistry.googleapis.com",
				"secretmanager.googleapis.com",
				"iap.googleapis.com",
			}
			for _, api := range apis {
				fmt.Printf("  Enabling %s...\n", api)
				run("gcloud", "services", "enable", api, "--project="+p.GCPProject)
			}

			// 4. Resources
			fmt.Println("\n[4/5] Resources")
			fmt.Println("  Creating Firestore database...")
			exec.Command("gcloud", "firestore", "databases", "create",
				"--project="+p.GCPProject, "--location=nam5", "--type=firestore-native").Run()
			fmt.Println("  Creating Artifact Registry repository...")
			exec.Command("gcloud", "artifacts", "repositories", "create", "cloud-run-source-deploy",
				"--project="+p.GCPProject, "--repository-format=docker",
				"--location="+p.Region, "--description=Cloud Run source deploy images").Run()

			// 5. OAuth
			fmt.Println("\n[5/5] OAuth")
			fmt.Println("  Configuring consent screen...")
			exec.Command("gcloud", "iap", "oauth-brands", "create",
				"--application_title="+p.Name, "--support_email="+email,
				"--project="+p.GCPProject).Run()

			// Validate credentials
			k := &config.KeychainResolver{}
			clientID, _ := k.Get(p.Name, "google-client-id")
			clientSecret, _ := k.Get(p.Name, "google-client-secret")

			if clientID != "" && clientSecret != "" {
				fmt.Println("  google-client-id found in keychain")
				fmt.Println("  google-client-secret found in keychain")
				fmt.Println("\n  OAuth credentials configured.")
			} else {
				fmt.Println("\n  ACTION REQUIRED:")
				fmt.Printf("\n  1. Create a Web OAuth client in Cloud Console:\n")
				fmt.Printf("     https://console.cloud.google.com/apis/credentials?project=%s\n", p.GCPProject)
				fmt.Println("\n  2. Download credentials JSON and import:")
				fmt.Println("     ab secret import ~/Downloads/client_secret_*.json")
				fmt.Printf("\n  3. Re-run: ab provision %s\n", email)
			}

			fmt.Println("\n  Required redirect URIs:")
			for _, uri := range p.redirectURIs() {
				fmt.Printf("    %s\n", uri)
			}

			fmt.Println("\n================================================")
			fmt.Println("Provisioning complete.")
			fmt.Println("\nNext steps:")
			if clientID == "" {
				fmt.Println("  1. Create OAuth client and: ab secret import <creds.json>")
				fmt.Printf("  2. ab provision %s  (re-run to verify)\n", email)
				fmt.Println("  3. ab deploy")
			} else {
				fmt.Println("  1. ab deploy")
			}
			fmt.Println("================================================")
			return nil
		},
	}
}

func dockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker <up|down|logs>",
		Short: "Local Docker container management",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			compose := "deploy/docker-compose.yml"
			if _, err := os.Stat(compose); err != nil {
				return fmt.Errorf("no %s found", compose)
			}
			switch args[0] {
			case "up":
				return run("docker", "compose", "-f", compose, "up", "-d", "--build")
			case "down":
				return run("docker", "compose", "-f", compose, "down")
			case "logs":
				return run("docker", "compose", "-f", compose, "logs", "-f")
			default:
				return fmt.Errorf("unknown docker command: %s (use up, down, or logs)", args[0])
			}
		},
	}
	return cmd
}
