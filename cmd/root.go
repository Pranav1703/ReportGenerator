package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"weeklyReportGenerator/config"
	gh "weeklyReportGenerator/github"
	"weeklyReportGenerator/llm"
	"weeklyReportGenerator/notify"
	"weeklyReportGenerator/report"

	"github.com/spf13/cobra"
)

var (
	weekOffset int
	startDate  string
	endDate    string
	outputDir  string
	emailTo    string
)

var rootCmd = &cobra.Command{
	Use:   "weeklyReportGenerator",
	Short: "Generate weekly tech reports from GitHub repos",
	Long:  "A tool that scans GitHub repos and generates PDF reports with AI-summarized weekly activity.",
	RunE:  run,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func init() {
	rootCmd.Flags().IntVar(&weekOffset, "week-offset", 0, "Weeks back from current (0=this week, 1=last week)")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Override start date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "Override end date (YYYY-MM-DD)")
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

	since, until, err := calculateWeekRange(weekOffset, startDate, endDate)
	if err != nil {
		return fmt.Errorf("date error: %w", err)
	}

	fmt.Printf("Generating report for %s to %s\n\n", since.Format("2006-01-02"), until.Format("2006-01-02"))

	filename := fmt.Sprintf("report-%s-to-%s.pdf",
		since.Format("2006-01-02"),
		until.Format("2006-01-02"))
	outputPath := fmt.Sprintf("%s/%s", cfg.OutputDir, filename)

	if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
		fmt.Printf("Report already exists, reusing: %s\n", outputPath)
		if err := sendEmail(cfg, outputPath, since, until); err != nil {
			log.Printf("Warning: %v", err)
		}
		return nil
	}

	ghClient := gh.NewClient(cfg.GitHubToken)
	llmClient := llm.NewClient(cfg.OpenRouterKey, cfg.LLMModel)
	ctx := context.Background()

	var summaries []report.RepoSummary

	for _, repoSlug := range cfg.GitHubRepos {
		owner, repo, err := gh.ParseRepoSlug(repoSlug)
		if err != nil {
			log.Printf("Skipping invalid repo %q: %v", repoSlug, err)
			continue
		}

		fmt.Printf("Fetching commits from %s...\n", repoSlug)
		commits, err := ghClient.GetWeeklyCommits(ctx, owner, repo, since, until)
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

		fmt.Printf("Generating AI summary for %s...\n", repoSlug)
		summary, err := llmClient.Summarize(ctx, repoSlug, commitTexts)
		if err != nil {
			log.Printf("Error generating summary for %s: %v", repoSlug, err)
			summary = fmt.Sprintf("[Summary generation failed: %v]\n\nCommits this week:\n", err)
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
	if err := report.Generate(summaries, since, until, outputPath); err != nil {
		return fmt.Errorf("error generating PDF: %w", err)
	}

	fmt.Printf("Report saved to: %s\n", outputPath)

	if err := sendEmail(cfg, outputPath, since, until); err != nil {
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

func sendEmail(cfg *config.Config, outputPath string, since, until time.Time) error {
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

	subject := fmt.Sprintf("Weekly Tech Team Report (%s to %s)",
		since.Format("2006-01-02"), until.Format("2006-01-02"))
	if err := notify.SendReport(smtpCfg, outputPath, subject, recipients); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Printf("Report emailed to: %s\n", strings.Join(recipients, ", "))
	return nil
}

func calculateWeekRange(offset int, startStr, endStr string) (time.Time, time.Time, error) {
	if startStr != "" && endStr != "" {
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

	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Saturday {
		weekday = 7
	}

	daysFromMonday := int(weekday) - int(time.Monday)
	monday := now.AddDate(0, 0, -daysFromMonday-offset*7)
	friday := monday.AddDate(0, 0, 4)

	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
	friday = time.Date(friday.Year(), friday.Month(), friday.Day(), 23, 59, 59, 0, friday.Location())

	return monday, friday, nil
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
