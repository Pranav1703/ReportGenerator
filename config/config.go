package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	GitHubToken   string
	GitHubRepos   []string
	OpenRouterKey string
	LLMModel      string
	OutputDir     string
}

func Load() (*Config, error) {
	godotenv.Load()

repos := strings.Split(os.Getenv("GITHUB_REPOS"), ",")
	for i := range repos {
		repos[i] = strings.TrimSpace(repos[i])
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "nvidia/nemotron-3-ultra-550b-a55b:free"
	}

	outputDir := os.Getenv("REPORT_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "reports"
	}

	return &Config{
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
		GitHubRepos:   repos,
		OpenRouterKey: os.Getenv("OPENROUTER_API_KEY"),
		LLMModel:      model,
		OutputDir:     outputDir,
	}, nil
}

func (c *Config) Validate() error {
	if c.GitHubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN is required")
	}
	if len(c.GitHubRepos) == 0 || c.GitHubRepos[0] == "" {
		return fmt.Errorf("GITHUB_REPOS is required (comma-separated owner/repo)")
	}
	if c.OpenRouterKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is required")
	}
	return nil
}
