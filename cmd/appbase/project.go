package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// project holds the merged config from app.yaml and app.json in cwd.
type project struct {
	Name          string
	GCPProject    string
	Region        string
	Port          int
	URLs          []string
	GCPAPIs       []string       // additional GCP APIs from app.yaml gcp.apis
	SchedulerJobs []schedulerJob // Cloud Scheduler jobs from app.yaml gcp.scheduler
}

type schedulerJob struct {
	Name     string
	Schedule string
	Path     string
	Method   string
	Headers  map[string]string
}

// loadProject reads project config from cwd.
// Tries app.yaml first, falls back to app.json.
func loadProject() (*project, error) {
	if p, err := loadFromYAML("app.yaml"); err == nil {
		return p, nil
	}
	if p, err := loadFromJSON("app.json"); err == nil {
		return p, nil
	}
	return nil, fmt.Errorf("no app.yaml or app.json found in current directory")
}

// mustLoadProject loads project config or exits with an error.
func mustLoadProject() *project {
	p, err := loadProject()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run 'ab init' to create a project config.")
		os.Exit(1)
	}
	return p
}

func loadFromYAML(path string) (*project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
		Store struct {
			GCPProject string `yaml:"gcp_project"`
		} `yaml:"store"`
		GCP struct {
			APIs      []string `yaml:"apis"`
			Scheduler []struct {
				Name     string            `yaml:"name"`
				Schedule string            `yaml:"schedule"`
				Path     string            `yaml:"path"`
				Method   string            `yaml:"method"`
				Headers  map[string]string `yaml:"headers"`
			} `yaml:"scheduler"`
		} `yaml:"gcp"`
		Environments map[string]struct {
			URL   string `yaml:"url"`
			Store struct {
				GCPProject string `yaml:"gcp_project"`
			} `yaml:"store"`
		} `yaml:"environments"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	p := &project{
		Name: cfg.Name,
		Port: cfg.Port,
	}
	if p.Port == 0 {
		p.Port = 3000
	}
	// Find GCP project from production env or top-level store
	if cfg.Store.GCPProject != "" {
		p.GCPProject = cfg.Store.GCPProject
	}
	for envName, env := range cfg.Environments {
		if env.URL != "" {
			p.URLs = append(p.URLs, env.URL)
		}
		if env.Store.GCPProject != "" && (envName == "production" || p.GCPProject == "") {
			p.GCPProject = env.Store.GCPProject
		}
	}
	// Read region from app.json if present (app.yaml doesn't have a top-level region)
	if rp, err := loadFromJSON("app.json"); err == nil {
		p.Region = rp.Region
	}
	if p.Region == "" {
		p.Region = "us-central1"
	}
	p.GCPAPIs = cfg.GCP.APIs
	for _, j := range cfg.GCP.Scheduler {
		method := j.Method
		if method == "" {
			method = "POST"
		}
		p.SchedulerJobs = append(p.SchedulerJobs, schedulerJob{
			Name:     j.Name,
			Schedule: j.Schedule,
			Path:     j.Path,
			Method:   method,
			Headers:  j.Headers,
		})
	}
	return p, nil
}

func loadFromJSON(path string) (*project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Name       string   `json:"name"`
		GCPProject string   `json:"gcpProject"`
		Region     string   `json:"region"`
		URLs       []string `json:"urls"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	p := &project{
		Name:       cfg.Name,
		GCPProject: cfg.GCPProject,
		Region:     cfg.Region,
		URLs:       cfg.URLs,
		Port:       3000,
	}
	if p.Region == "" {
		p.Region = "us-central1"
	}
	return p, nil
}

// redirectURIs returns OAuth redirect URIs derived from the project URLs.
func (p *project) redirectURIs() []string {
	var uris []string
	for _, u := range p.URLs {
		u = strings.TrimRight(u, "/")
		uris = append(uris, u+"/api/auth/callback")
	}
	if len(uris) == 0 {
		uris = append(uris, fmt.Sprintf("http://localhost:%d/api/auth/callback", p.Port))
	}
	return uris
}
