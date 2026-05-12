package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Name    string            `yaml:"name"`
	Port    int               `yaml:"port"`
	Source  string            `yaml:"source"`   // "github" или "static"
	Repo    string            `yaml:"repo"`     // Для github
	ArchMap map[string]string `yaml:"arch_map"` // Для github (path -> search_term)
	Links   map[string]string `yaml:"links"`    // Для static (path -> direct_url)
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

// Хендлер для GitHub: ищет файл в последнем релизе
func handleGithub(app AppConfig, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	searchTerm, exists := app.ArchMap[path]
	if !exists {
		searchTerm = path
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", app.Repo))
	if err != nil {
		http.Error(w, "GitHub API Error", 502)
		return
	}
	defer resp.Body.Close()

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		http.Error(w, "JSON Decode Error", 500)
		return
	}

	for _, asset := range rel.Assets {
		if strings.Contains(strings.ToLower(asset.Name), strings.ToLower(searchTerm)) {
			http.Redirect(w, r, asset.URL, http.StatusFound)
			return
		}
	}
	http.Error(w, "Asset not found in GitHub release", 404)
}

// Хендлер для статических ссылок: просто редиректит по ключу
func handleStatic(app AppConfig, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	url, exists := app.Links[path]
	if !exists {
		http.Error(w, "Link not found for this architecture", 404)
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func main() {
	// 1. Загрузка конфига
	file, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("Error reading config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(file, &cfg); err != nil {
		log.Fatalf("Error parsing YAML: %v", err)
	}

	// 2. Запуск серверов для каждого приложения
	for _, app := range cfg.Apps {
		app := app // замыкание
		go func() {
			mux := http.NewServeMux()

			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if app.Source == "github" {
					handleGithub(app, w, r)
				} else {
					handleStatic(app, w, r)
				}
			})

			fmt.Printf("✅ [%s] mode:%s on :%d\n", app.Name, app.Source, app.Port)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", app.Port), mux); err != nil {
				log.Printf("Server %s failed: %v", app.Name, err)
			}
		}()
	}

	// Блокируем главный поток
	select {}
}
