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
		Use:   "deploy [target-name]",
		Short: "Deploy to Cloud Run",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv("NONO_CAP_FILE") != "" {
				return fmt.Errorf("deploy cannot run inside a nono sandbox — cloud credentials are blocked by design.\nRun outside the sandbox: ./dev deploy")
			}

			targetName := ""
			if len(args) > 0 {
				targetName = args[0]
			}

			// Try target-based deploy first, fall back to legacy
			cfg, err := loadAppConfig()
			if err == nil && (len(cfg.Targets) > 0 || targetName != "") {
				return deployTarget(cfg, targetName)
			}

			// Legacy path: use project struct
			return deployLegacy()
		},
	}
}

func deployTarget(cfg *config.AppConfig, targetName string) error {
	target, err := cfg.Target(targetName)
	if err != nil {
		return err
	}

	if target.Project == "" {
		return fmt.Errorf("target has no project set")
	}

	region := target.Region
	if region == "" {
		region = "us-central1"
	}

	storeType := target.Store.Type
	if v := os.Getenv("STORE_TYPE"); v != "" {
		storeType = v
	}
	if storeType == "" {
		storeType = "firestore"
	}

	displayName := targetName
	if displayName == "" {
		displayName = "(default)"
	}
	fmt.Printf("Deploying %s → %s (project: %s, region: %s, store: %s)...\n",
		cfg.Name, displayName, target.Project, region, storeType)

	if storeType == "sqlite" {
		fmt.Println("\n  WARNING: Using SQLite on Cloud Run. Data will be lost on cold starts.")
		fmt.Println("  Use store.type: firestore for production.")
		fmt.Println()
	}

	// Resolve secrets from keychain
	k := &config.KeychainResolver{}
	clientID := target.Auth.ClientID
	clientSecret := target.Auth.ClientSecret

	// Resolve ${secret:name} references
	if strings.HasPrefix(clientID, "${secret:") {
		secretName := clientID[9 : len(clientID)-1]
		clientID, _ = k.Get(cfg.Name, secretName)
	}
	if strings.HasPrefix(clientSecret, "${secret:") {
		secretName := clientSecret[9 : len(clientSecret)-1]
		clientSecret, _ = k.Get(cfg.Name, secretName)
	}

	// Build env vars
	envVars := fmt.Sprintf("STORE_TYPE=%s,GOOGLE_CLOUD_PROJECT=%s", storeType, target.Project)
	if v := os.Getenv("ALLOWED_USERS"); v != "" {
		envVars += ",ALLOWED_USERS=" + v
	}

	// Add target-specific env vars (resolve secrets)
	for envKey, envVal := range target.Env {
		resolved := envVal
		if strings.HasPrefix(envVal, "${secret:") && strings.HasSuffix(envVal, "}") {
			secretName := envVal[9 : len(envVal)-1]
			if v, err := k.Get(cfg.Name, secretName); err == nil {
				resolved = v
			}
		}
		envVars += "," + envKey + "=" + resolved
	}

	// Push auth secrets to Secret Manager
	var secretsFlag string
	if clientID != "" {
		fmt.Println("  Pushing google-client-id to Secret Manager...")
		pushSecret(target.Project, "google-client-id", clientID)
		secretsFlag = "GOOGLE_CLIENT_ID=google-client-id:latest"
	}
	if clientSecret != "" {
		fmt.Println("  Pushing google-client-secret to Secret Manager...")
		pushSecret(target.Project, "google-client-secret", clientSecret)
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
		"run", "deploy", cfg.Name,
		"--source", ".",
		"--project=" + target.Project,
		"--region=" + region,
		"--allow-unauthenticated",
		"--clear-base-image",
		"--set-env-vars=" + envVars,
	}
	if secretsFlag != "" {
		gcloudArgs = append(gcloudArgs, "--set-secrets="+secretsFlag)
	}
	if target.Timeout > 0 {
		gcloudArgs = append(gcloudArgs, fmt.Sprintf("--timeout=%d", target.Timeout))
	}

	if err := run("gcloud", gcloudArgs...); err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	// Capture service URL
	out, err := exec.Command("gcloud", "run", "services", "describe", cfg.Name,
		"--project="+target.Project,
		"--region="+region,
		"--format=value(status.url)").Output()
	if err == nil {
		serviceURL := strings.TrimSpace(string(out))
		if serviceURL != "" {
			fmt.Printf("\nService URL: %s\n", serviceURL)
		}
	}

	return nil
}

func deployLegacy() error {
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

	k := &config.KeychainResolver{}
	clientID, _ := k.Get(p.Name, "google-client-id")
	clientSecret, _ := k.Get(p.Name, "google-client-secret")

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
}

func pushSecret(gcpProject, name, value string) {
	err := exec.Command("gcloud", "secrets", "describe", name,
		"--project="+gcpProject).Run()
	if err == nil {
		cmd := exec.Command("gcloud", "secrets", "versions", "add", name,
			"--project="+gcpProject, "--data-file=-")
		cmd.Stdin = strings.NewReader(value)
		cmd.Run()
	} else {
		cmd := exec.Command("gcloud", "secrets", "create", name,
			"--project="+gcpProject, "--replication-policy=automatic", "--data-file=-")
		cmd.Stdin = strings.NewReader(value)
		cmd.Run()
	}
}

func provisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision [target-name|support-email]",
		Short: "Full GCP setup (project, billing, APIs, resources, OAuth)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Try target-based provision
			cfg, cfgErr := loadAppConfig()
			if cfgErr == nil && len(cfg.Targets) > 0 {
				targetName := ""
				if len(args) > 0 {
					targetName = args[0]
				}
				return provisionTarget(cfg, targetName)
			}

			// Legacy path: positional email argument
			if len(args) == 0 {
				return fmt.Errorf("usage: appbase provision <support-email>\n\nOr configure targets in app.yaml for named provisioning.")
			}

			return provisionLegacy(args[0])
		},
	}
}

func provisionTarget(cfg *config.AppConfig, targetName string) error {
	target, err := cfg.Target(targetName)
	if err != nil {
		return err
	}

	if target.Project == "" {
		return fmt.Errorf("target has no project set")
	}

	email := target.SupportEmail
	if email == "" {
		return fmt.Errorf("target has no support_email set — add it to the target config in app.yaml")
	}

	region := target.Region
	if region == "" {
		region = "us-central1"
	}

	displayName := targetName
	if displayName == "" {
		displayName = "(default)"
	}

	fmt.Println("================================================")
	fmt.Printf("Provisioning %s → %s\n", cfg.Name, displayName)
	fmt.Printf("  Project: %s\n", target.Project)
	fmt.Printf("  Region:  %s\n", region)
	fmt.Printf("  Contact: %s\n", email)
	fmt.Println("================================================")
	fmt.Println()

	// 1. Project
	fmt.Println("[1/5] Project")
	run("gcloud", "projects", "describe", target.Project)
	run("gcloud", "config", "set", "project", target.Project)

	// 2. Billing
	fmt.Println("\n[2/5] Billing")
	out, _ := exec.Command("gcloud", "billing", "projects", "describe", target.Project,
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
	}
	apis = append(apis, cfg.GCP.APIs...)
	for _, api := range apis {
		fmt.Printf("  Enabling %s...\n", api)
		run("gcloud", "services", "enable", api, "--project="+target.Project)
	}

	// 4. Resources
	fmt.Println("\n[4/5] Resources")
	fmt.Println("  Creating Firestore database...")
	exec.Command("gcloud", "firestore", "databases", "create",
		"--project="+target.Project, "--location=nam5", "--type=firestore-native").Run()
	fmt.Println("  Creating Artifact Registry repository...")
	exec.Command("gcloud", "artifacts", "repositories", "create", "cloud-run-source-deploy",
		"--project="+target.Project, "--repository-format=docker",
		"--location="+region, "--description=Cloud Run source deploy images").Run()

	// Grant Secret Manager access
	fmt.Println("  Granting Secret Manager access to Cloud Run...")
	projNum, _ := exec.Command("gcloud", "projects", "describe", target.Project,
		"--format=value(projectNumber)").Output()
	if num := strings.TrimSpace(string(projNum)); num != "" {
		sa := num + "-compute@developer.gserviceaccount.com"
		exec.Command("gcloud", "projects", "add-iam-policy-binding", target.Project,
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
		"--application_title="+cfg.Name, "--support_email="+email,
		"--project="+target.Project).Run()

	k := &config.KeychainResolver{}
	clientID, _ := k.Get(cfg.Name, "google-client-id")
	clientSecret, _ := k.Get(cfg.Name, "google-client-secret")

	if clientID != "" && clientSecret != "" {
		fmt.Println("  google-client-id found in keychain")
		fmt.Println("  google-client-secret found in keychain")
		fmt.Println("\n  OAuth credentials configured.")
	} else {
		fmt.Println("\n  ACTION REQUIRED:")
		fmt.Printf("\n  1. Create a Web OAuth client in Cloud Console:\n")
		fmt.Printf("     https://console.cloud.google.com/apis/credentials?project=%s\n", target.Project)
		fmt.Println("\n  2. Download credentials JSON and import:")
		fmt.Println("     appbase secret import ~/Downloads/client_secret_*.json")
	}

	fmt.Println("\n================================================")
	fmt.Println("Provisioning complete.")
	if clientID == "" {
		fmt.Println("\nNext: import OAuth credentials, then: appbase deploy")
	} else {
		fmt.Println("\nNext: appbase deploy")
	}
	fmt.Println("================================================")
	return nil
}

func provisionLegacy(email string) error {
	p := mustLoadProject()

	if p.GCPProject == "" {
		return fmt.Errorf("gcpProject not set — run 'ab init' first")
	}

	fmt.Println("================================================")
	fmt.Printf("Provisioning GCP for %s\n", p.Name)
	fmt.Printf("  Project: %s\n", p.GCPProject)
	fmt.Printf("  Contact: %s\n", email)
	fmt.Println("================================================")
	fmt.Println()

	fmt.Println("[1/5] Project")
	run("gcloud", "projects", "describe", p.GCPProject)
	run("gcloud", "config", "set", "project", p.GCPProject)

	fmt.Println("\n[2/5] Billing")
	out, _ := exec.Command("gcloud", "billing", "projects", "describe", p.GCPProject,
		"--format=value(billingAccountName)").Output()
	if strings.TrimSpace(string(out)) != "" {
		fmt.Printf("  Billing already linked: %s\n", strings.TrimSpace(string(out)))
	} else {
		fmt.Println("  WARNING: No billing linked. Link manually or pass a billing account.")
	}

	fmt.Println("\n[3/5] APIs")
	apis := []string{
		"cloudbuild.googleapis.com",
		"run.googleapis.com",
		"firestore.googleapis.com",
		"artifactregistry.googleapis.com",
		"secretmanager.googleapis.com",
	}
	apis = append(apis, p.GCPAPIs...)
	for _, api := range apis {
		fmt.Printf("  Enabling %s...\n", api)
		run("gcloud", "services", "enable", api, "--project="+p.GCPProject)
	}

	fmt.Println("\n[4/5] Resources")
	fmt.Println("  Creating Firestore database...")
	exec.Command("gcloud", "firestore", "databases", "create",
		"--project="+p.GCPProject, "--location=nam5", "--type=firestore-native").Run()
	fmt.Println("  Creating Artifact Registry repository...")
	exec.Command("gcloud", "artifacts", "repositories", "create", "cloud-run-source-deploy",
		"--project="+p.GCPProject, "--repository-format=docker",
		"--location="+p.Region, "--description=Cloud Run source deploy images").Run()

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

	fmt.Println("\n[5/5] OAuth")
	fmt.Println("  Configuring consent screen...")
	exec.Command("gcloud", "iap", "oauth-brands", "create",
		"--application_title="+p.Name, "--support_email="+email,
		"--project="+p.GCPProject).Run()

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
