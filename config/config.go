package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	GitHubToken    string
	GitHubRepos    []string
	OpenRouterKey  string
	LLMModel       string
	LLMReduceModel string
	LLMChunkSize   int
	OutputDir      string
	SMTPHost       string
	SMTPPort       int
	SMTPUser       string
	SMTPPass       string
	SMTPFrom       string
}

func Load() (*Config, error) {
	godotenv.Load()

repos := strings.Split(os.Getenv("GITHUB_REPOS"), ",")
	for i := range repos {
		repos[i] = strings.TrimSpace(repos[i])
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "nvidia/nemotron-3-super-120b-a12b:free"
	}

	reduceModel := os.Getenv("LLM_REDUCE_MODEL")
	if reduceModel == "" {
		reduceModel = model
	}

	chunkSize := 15
	if v := os.Getenv("LLM_CHUNK_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			chunkSize = n
		}
	}

	outputDir := os.Getenv("REPORT_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "reports"
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}

	smtpPort := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			smtpPort = v
		}
	}

	return &Config{
		GitHubToken:    os.Getenv("GITHUB_TOKEN"),
		GitHubRepos:    repos,
		OpenRouterKey:  os.Getenv("OPENROUTER_API_KEY"),
		LLMModel:       model,
		LLMReduceModel: reduceModel,
		LLMChunkSize:   chunkSize,
		OutputDir:      outputDir,
		SMTPHost:      smtpHost,
		SMTPPort:      smtpPort,
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		SMTPFrom:      os.Getenv("SMTP_FROM"),
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
