package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"reportGenerator/config"
	gh "reportGenerator/github"
	"reportGenerator/llm"
	"reportGenerator/notify"
	"reportGenerator/report"

	"github.com/spf13/cobra"
)

var (
	reportType string
	startDate  string
	endDate    string
	outputDir  string
	emailTo    string
)

var rootCmd = &cobra.Command{
	Use:   "reportGenerator",
	Short: "Generate tech reports from GitHub repos",
	Long:  "A tool that scans GitHub repos and generates PDF reports with AI-summarized activity.",
	RunE:  run,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.Flags().StringVar(&reportType, "type", "weekly", "Report type: weekly or monthly")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&outputDir, "output", "", "Output directory override")
	rootCmd.Flags().StringVar(&emailTo, "email", "", "Send the generated report to these email address(es), comma-separated")
}

func run(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if outputDir != "" {
		cfg.OutputDir = outputDir
	}

	title, err := reportTitle(reportType)
	if err != nil {
		return err
	}

	since, until, err := calculateDateRange(startDate, endDate)
	if err != nil {
		return fmt.Errorf("date error: %w", err)
	}

	fmt.Printf("Generating %s for %s to %s\n\n", title, since.Format("2006-01-02"), until.Format("2006-01-02"))

	filename := fmt.Sprintf("report-%s-to-%s.pdf",
		since.Format("2006-01-02"),
		until.Format("2006-01-02"))
	outputPath := fmt.Sprintf("%s/%s", cfg.OutputDir, filename)

	if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
		fmt.Printf("Report already exists, reusing: %s\n", outputPath)
		if err := sendEmail(cfg, outputPath, title, since, until); err != nil {
			log.Printf("Warning: %v", err)
		}
		return nil
	}

	ghClient := gh.NewClient(cfg.GitHubToken)
	llmClient := llm.NewClient(cfg.OpenRouterKey, cfg.LLMModel, cfg.LLMReduceModel, cfg.LLMChunkSize)
	ctx := context.Background()

	var summaries []report.RepoSummary

	for _, repoSlug := range cfg.GitHubRepos {
		owner, repo, err := gh.ParseRepoSlug(repoSlug)
		if err != nil {
			log.Printf("Skipping invalid repo %q: %v", repoSlug, err)
			continue
		}

		fmt.Printf("Fetching commits from %s...\n", repoSlug)
		commits, err := ghClient.GetCommits(ctx, owner, repo, since, until)
		if err != nil {
			log.Printf("Error fetching commits for %s: %v", repoSlug, err)
			continue
		}

		if len(commits) == 0 {
			fmt.Printf("No commits found for %s, skipping\n", repoSlug)
			continue
		}

		fmt.Printf("Found %d commits, fetching diffs...\n", len(commits))

		commitTexts := make([]string, 0, len(commits))
		for _, c := range commits {
			files, err := ghClient.GetCommitFileChanges(ctx, owner, repo, c.SHA)
			if err != nil {
				fmt.Printf("  Warning: could not fetch changes for %s: %v\n", c.SHA[:8], err)
				commitTexts = append(commitTexts, c.Message)
				continue
			}

			commitTexts = append(commitTexts, formatCommit(c.Message, files))
		}

		if len(commitTexts) > cfg.LLMChunkSize {
			numChunks := (len(commitTexts) + cfg.LLMChunkSize - 1) / cfg.LLMChunkSize
			fmt.Printf("Splitting %d commits into %d chunks for %s...\n", len(commitTexts), numChunks, repoSlug)
		}

		fmt.Printf("Generating AI summary for %s...\n", repoSlug)
		summary, err := llmClient.Summarize(ctx, repoSlug, commitTexts)
		if err != nil {
			log.Printf("Error generating summary for %s: %v", repoSlug, err)
			summary = fmt.Sprintf("[Summary generation failed: %v]\n\nCommits:\n", err)
			for _, c := range commits {
				summary += fmt.Sprintf("- %s\n", c.Message)
			}
		}

		summaries = append(summaries, report.RepoSummary{
			RepoSlug: repoSlug,
			Summary:  summary,
		})
		fmt.Println()
	}

	if len(summaries) == 0 {
		return fmt.Errorf("no summaries to generate report from")
	}

	if err := report.EnsureOutputDir(cfg.OutputDir); err != nil {
		return fmt.Errorf("error creating output directory: %w", err)
	}

	fmt.Printf("Generating PDF report...\n")
	if err := report.Generate(summaries, since, until, title, outputPath); err != nil {
		return fmt.Errorf("error generating PDF: %w", err)
	}

	fmt.Printf("Report saved to: %s\n", outputPath)

	if err := sendEmail(cfg, outputPath, title, since, until); err != nil {
		log.Printf("Warning: %v", err)
	}

	return nil
}

func splitRecipients(raw string) []string {
	parts := strings.Split(raw, ",")
	recipients := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			recipients = append(recipients, p)
		}
	}
	return recipients
}

func sendEmail(cfg *config.Config, outputPath, title string, since, until time.Time) error {
	if emailTo == "" {
		return nil
	}

	recipients := splitRecipients(emailTo)
	if len(recipients) == 0 {
		return fmt.Errorf("no valid recipients in --email value %q", emailTo)
	}

	smtpCfg := notify.SMTPConfig{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	}
	if smtpCfg.User == "" || smtpCfg.Pass == "" || smtpCfg.From == "" {
		return fmt.Errorf("SMTP not configured (SMTP_USER, SMTP_PASS, SMTP_FROM missing)")
	}

	subject := fmt.Sprintf("%s (%s to %s)",
		title, since.Format("2006-01-02"), until.Format("2006-01-02"))
	if err := notify.SendReport(smtpCfg, outputPath, subject, recipients); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Printf("Report emailed to: %s\n", strings.Join(recipients, ", "))
	return nil
}

func reportTitle(reportType string) (string, error) {
	switch strings.ToLower(reportType) {
	case "weekly":
		return "Weekly Tech Team Report", nil
	case "monthly":
		return "Monthly Tech Team Report", nil
	default:
		return "", fmt.Errorf("invalid report type %q: must be \"weekly\" or \"monthly\"", reportType)
	}
}

func calculateDateRange(startStr, endStr string) (time.Time, time.Time, error) {
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("please provide a date range using --start-date and --end-date (YYYY-MM-DD)")
	}

	since, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start-date: %w", err)
	}
	until, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end-date: %w", err)
	}

	return since, until.Add(24*time.Hour - time.Second), nil
}

func formatCommit(msg string, files []gh.FileChange) string {
	if len(files) == 0 {
		return msg
	}

	parts := make([]string, 0, len(files))
	for _, f := range files {
		parts = append(parts, fmt.Sprintf("%s %s (+%d -%d)", f.Status, f.Filename, f.Additions, f.Deletions))
	}
	return fmt.Sprintf("%s\n  %s", msg, strings.Join(parts, ", "))
}
