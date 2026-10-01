package llm

import (
	"context"
	"fmt"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type Client struct {
	sdk         *openrouter.OpenRouter
	model       string
	reduceModel string
	chunkSize   int
}

func NewClient(apiKey, model, reduceModel string, chunkSize int) *Client {
	sdk := openrouter.New(
		openrouter.WithSecurity(apiKey),
		openrouter.WithTimeout(300*time.Second),
	)
	if reduceModel == "" {
		reduceModel = model
	}
	if chunkSize <= 0 {
		chunkSize = 15
	}
	return &Client{
		sdk:         sdk,
		model:       model,
		reduceModel: reduceModel,
		chunkSize:   chunkSize,
	}
}

func (c *Client) Summarize(ctx context.Context, repoSlug string, commits []string) (string, error) {
	if len(commits) <= c.chunkSize {
		return c.retry(func() (string, error) {
			return c.doSummarize(ctx, repoSlug, commits)
		})
	}

	chunks := chunkCommits(commits, c.chunkSize)
	chunkSummaries := make([]string, 0, len(chunks))

	for i, chunk := range chunks {
		sum, err := c.retry(func() (string, error) {
			return c.summarizeChunk(ctx, repoSlug, chunk, i+1, len(chunks))
		})
		if err != nil {
			return "", fmt.Errorf("summarizing chunk %d/%d: %w", i+1, len(chunks), err)
		}
		chunkSummaries = append(chunkSummaries, sum)
	}

	return c.retry(func() (string, error) {
		return c.mergeSummaries(ctx, repoSlug, chunkSummaries)
	})
}

func (c *Client) retry(fn func() (string, error)) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}

		result, err := fn()
		if err == nil && result != "" {
			return result, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("LLM failed after 3 attempts: %w", lastErr)
}

func (c *Client) doSummarize(ctx context.Context, repoSlug string, commits []string) (string, error) {
	prompt := fmt.Sprintf(`You are a tech lead writing a progress report for a non-technical reader.
	Summarize the work done in the repository "%s" during this reporting period thoroughly and accurately.
	Group the work into clear sections using markdown headings (e.g. "### New Features", "### Bug Fixes", "### Improvements", "### Infrastructure").
	Under each section, list every meaningful change as a bullet point.
	For each bullet, write a plain-English explanation of what changed, why it matters, and what impact it has.
	Do not compress unrelated changes into a single bullet; cover the work in detail.
	Avoid deep technical jargon, but keep it substantive and informative rather than superficial.

	Commits and their changes:
	%s`, repoSlug, truncate(formatCommits(commits)))

	return c.send(ctx, c.model, prompt)
}

func (c *Client) summarizeChunk(ctx context.Context, repoSlug string, commits []string, idx, total int) (string, error) {
	prompt := fmt.Sprintf(`You are a tech lead summarizing a portion of a progress report for a non-technical reader.
	This is chunk %d of %d for the repository "%s".
	Summarize ONLY the work described in the commits below as concise, plain-English bullet points.
	Group them into short markdown sections (e.g. "### New Features", "### Bug Fixes", "### Improvements", "### Infrastructure").
	For each bullet, briefly state what changed and why it matters.
	Be terse: this output will be merged with other chunks, so do not add introductions or conclusions.

	Commits and their changes:
	%s`, idx, total, repoSlug, truncate(formatCommits(commits)))

	return c.send(ctx, c.model, prompt)
}

func (c *Client) mergeSummaries(ctx context.Context, repoSlug string, chunks []string) (string, error) {
	var b string
	for i, chunk := range chunks {
		b += fmt.Sprintf("=== Chunk %d ===\n%s\n\n", i+1, chunk)
	}

	prompt := fmt.Sprintf(`You are a tech lead writing a final progress report for a non-technical reader.
	Below are summaries of separate chunks of work from the repository "%s".
	Merge them into a single coherent report.
	Group the work into clear sections using markdown headings (e.g. "### New Features", "### Bug Fixes", "### Improvements", "### Infrastructure").
	Under each section, list every meaningful change as a bullet point.
	For each bullet, write a plain-English explanation of what changed, why it matters, and what impact it has.
	Remove duplicates and combine related items from different chunks where appropriate.
	Avoid deep technical jargon, but keep it substantive and informative rather than superficial.

	Chunk summaries:
	%s`, repoSlug, truncate(b))

	return c.send(ctx, c.reduceModel, prompt)
}

func (c *Client) send(ctx context.Context, model, prompt string) (string, error) {
	res, err := c.sdk.Chat.Send(ctx, components.ChatRequest{
		Model: openrouter.Pointer(model),
		Messages: []components.ChatMessages{
			components.CreateChatMessagesUser(
				components.ChatUserMessage{
					Role: components.ChatUserMessageRoleUser,
					Content: components.CreateChatUserMessageContentStr(
						prompt,
					),
				},
			),
		},
		Temperature: optionalnullable.From(openrouter.Pointer(0.3)),
	}, nil)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}

	if res == nil || res.ChatResult == nil || len(res.ChatResult.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	content := res.ChatResult.Choices[0].Message.Content
	if content == nil {
		return "", fmt.Errorf("empty content in response")
	}

	contentVal, isSet := content.Get()
	if !isSet || contentVal == nil {
		return "", fmt.Errorf("content not set in response")
	}

	if contentVal.Str != nil {
		return *contentVal.Str, nil
	}

	return "", fmt.Errorf("unexpected content type in response")
}

func truncate(s string) string {
	const maxInputChars = 20000
	if len(s) > maxInputChars {
		return s[:maxInputChars] + "\n... [input truncated]"
	}
	return s
}

func formatCommits(commits []string) string {
	result := ""
	for i, c := range commits {
		result += fmt.Sprintf("%d. %s\n", i+1, c)
	}
	return result
}

func chunkCommits(commits []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(commits); i += size {
		end := i + size
		if end > len(commits) {
			end = len(commits)
		}
		chunks = append(chunks, commits[i:end])
	}
	return chunks
}
