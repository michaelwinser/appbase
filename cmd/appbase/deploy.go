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
			if os.Getenv("NONO_CAP_FILE") != "" {
				return fmt.Errorf("deploy cannot run inside a nono sandbox — cloud credentials are blocked by design.\nRun outside the sandbox: ./dev deploy")
			}

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
			serviceURL := ""
			if err == nil {
				serviceURL = strings.TrimSpace(string(out))
				if serviceURL != "" {
					fmt.Printf("\nService URL: %s\n", serviceURL)
				}
			}

			// Set up Cloud Scheduler jobs
			if len(p.SchedulerJobs) > 0 && serviceURL != "" {
				if err := setupSchedulerJobs(p, serviceURL); err != nil {
					fmt.Printf("\nWARNING: scheduler setup failed: %v\n", err)
					fmt.Println("You can retry with: appbase deploy")
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

			// 3. APIs — infrastructure + app-specific from app.yaml gcp.apis
			fmt.Println("\n[3/5] APIs")
			apis := []string{
				"cloudbuild.googleapis.com",       // Cloud Build (source deploy)
				"run.googleapis.com",              // Cloud Run
				"firestore.googleapis.com",        // Firestore database
				"artifactregistry.googleapis.com", // Container image registry
				"secretmanager.googleapis.com",    // Secret storage
			}
			apis = append(apis, p.GCPAPIs...)
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

			// Grant Secret Manager access to Cloud Run service account
			fmt.Println("  Granting Secret Manager access to Cloud Run...")
			projNum, _ := exec.Command("gcloud", "projects", "describe", p.GCPProject,
				"--format=value(projectNumber)").Output()
			if num := strings.TrimSpace(string(projNum)); num != "" {
				sa := num + "-compute@developer.gserviceaccount.com"
				exec.Command("gcloud", "projects", "add-iam-policy-binding", p.GCPProject,
					"--member=serviceAccount:"+sa,
					"--role=roles/secretmanager.secretAccessor",
					"--condition=None",
					"--quiet").Run()
				fmt.Printf("  Granted secretmanager.secretAccessor to %s\n", sa)
			}

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

func setupSchedulerJobs(p *project, serviceURL string) error {
	fmt.Printf("\nSetting up %d Cloud Scheduler job(s)...\n", len(p.SchedulerJobs))

	// Ensure scheduler-invoker service account exists
	saName := p.Name + "-scheduler"
	saEmail := saName + "@" + p.GCPProject + ".iam.gserviceaccount.com"

	fmt.Printf("  Creating service account %s...\n", saName)
	exec.Command("gcloud", "iam", "service-accounts", "create", saName,
		"--project="+p.GCPProject,
		"--display-name=Cloud Scheduler invoker for "+p.Name,
		"--quiet").Run() // ignore error if already exists

	// Grant Cloud Run invoker role
	fmt.Println("  Granting Cloud Run invoker role...")
	exec.Command("gcloud", "run", "services", "add-iam-policy-binding", p.Name,
		"--project="+p.GCPProject,
		"--region="+p.Region,
		"--member=serviceAccount:"+saEmail,
		"--role=roles/run.invoker",
		"--quiet").Run()

	// Create/update each job
	for _, job := range p.SchedulerJobs {
		jobName := p.Name + "-" + job.Name
		targetURL := strings.TrimRight(serviceURL, "/") + job.Path

		fmt.Printf("  Job %s: %s %s [%s]\n", jobName, job.Method, job.Path, job.Schedule)

		// Delete existing job (idempotent recreate)
		exec.Command("gcloud", "scheduler", "jobs", "delete", jobName,
			"--project="+p.GCPProject,
			"--location="+p.Region,
			"--quiet").Run()

		// Build create command
		createArgs := []string{
			"scheduler", "jobs", "create", "http", jobName,
			"--project=" + p.GCPProject,
			"--location=" + p.Region,
			"--schedule=" + job.Schedule,
			"--uri=" + targetURL,
			"--http-method=" + job.Method,
			"--oidc-service-account-email=" + saEmail,
			"--oidc-token-audience=" + serviceURL,
			"--quiet",
		}

		// Add headers
		for k, v := range job.Headers {
			createArgs = append(createArgs, "--headers="+k+"="+v)
		}

		if err := run("gcloud", createArgs...); err != nil {
			return fmt.Errorf("creating job %s: %w", jobName, err)
		}
	}

	fmt.Println("  Scheduler jobs configured.")
	return nil
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
