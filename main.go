package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Name    string            `yaml:"name"`
	Bind    string            `yaml:"bind"`
	Port    int               `yaml:"port"`
	Source  string            `yaml:"source"`
	Repo    string            `yaml:"repo"`
	ArchMap map[string]string `yaml:"arch_map"`
	Links   map[string]string `yaml:"links"`
}

type Config struct {
	Apps []AppConfig `yaml:"apps"`
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type Release struct {
	Assets []Asset `json:"assets"`
}

var client = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

func proxyURL(targetURL string, w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		http.Error(w, "Bad upstream URL", 500)
		return
	}

	for key, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Upstream request failed", 502)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		w.Header().Del(key)
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[%s] stream error: %v", targetURL, err)
	}
}

func handleGithub(app AppConfig, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	searchTerm, exists := app.ArchMap[path]
	if !exists {
		searchTerm = path
	}

	apiClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := apiClient.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", app.Repo))
	if err != nil {
		http.Error(w, "GitHub API error", 502)
		return
	}
	defer resp.Body.Close()

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		http.Error(w, "JSON decode error", 500)
		return
	}

	for _, asset := range rel.Assets {
		if strings.Contains(strings.ToLower(asset.Name), strings.ToLower(searchTerm)) {
			log.Printf("[%s] proxying GitHub asset: %s", app.Name, asset.URL)
			proxyURL(asset.URL, w, r)
			return
		}
	}

	http.Error(w, "Asset not found in GitHub release", 404)
}

func handleStatic(app AppConfig, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	targetURL, exists := app.Links[path]
	if !exists {
		http.Error(w, "Link not found for this path", 404)
		return
	}

	log.Printf("[%s] proxying static: %s -> %s", app.Name, path, targetURL)
	proxyURL(targetURL, w, r)
}

func main() {
	file, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(file, &cfg); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	for _, app := range cfg.Apps {
		app := app
		go func() {
			mux := http.NewServeMux()

			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if app.Source == "github" {
					handleGithub(app, w, r)
				} else {
					handleStatic(app, w, r)
				}
			})

			bind := app.Bind
			if bind == "" {
				bind = "127.0.0.1"
			}
			addr := fmt.Sprintf("%s:%d", bind, app.Port)
			fmt.Printf("✅ [%s] mode:%s on %s\n", app.Name, app.Source, addr)
			if err := http.ListenAndServe(addr, mux); err != nil {
				log.Printf("Server %s failed: %v", app.Name, err)
			}
		}()
	}

	select {}
}