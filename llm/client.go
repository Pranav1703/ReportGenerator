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
	sdk   *openrouter.OpenRouter
	model string
}

func NewClient(apiKey, model string) *Client {
	sdk := openrouter.New(
		openrouter.WithSecurity(apiKey),
		openrouter.WithTimeout(300*time.Second),
	)
	return &Client{
		sdk:   sdk,
		model: model,
	}
}

func (c *Client) Summarize(ctx context.Context, repoSlug string, commits []string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}

		result, err := c.doSummarize(ctx, repoSlug, commits)
		if err == nil && result != "" {
			return result, nil
		}
		lastErr = err
	}

	return "", fmt.Errorf("LLM failed after 3 attempts: %w", lastErr)
}

func (c *Client) doSummarize(ctx context.Context, repoSlug string, commits []string) (string, error) {
	commitsText := formatCommits(commits)
	const maxInputChars = 20000
	if len(commitsText) > maxInputChars {
		commitsText = commitsText[:maxInputChars] + "\n... [input truncated]"
	}

	prompt := fmt.Sprintf(`You are a tech lead writing a progress report (weekly or monthly) for a non-technical reader.
	Summarize the work done in the repository "%s" during this reporting period thoroughly and accurately.
	Group the work into clear sections using markdown headings (e.g. "### New Features", "### Bug Fixes", "### Improvements", "### Infrastructure").
	Under each section, list every meaningful change as a bullet point.
	For each bullet, write a plain-English explanation of what changed, why it matters, and what impact it has.
	Do not compress unrelated changes into a single bullet; cover the work in detail.
	Avoid deep technical jargon, but keep it substantive and informative rather than superficial.

	Commits and their changes:
	%s`, repoSlug, commitsText)

	res, err := c.sdk.Chat.Send(ctx, components.ChatRequest{
		Model: openrouter.Pointer(c.model),
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

func formatCommits(commits []string) string {
	result := ""
	for i, c := range commits {
		result += fmt.Sprintf("%d. %s\n", i+1, c)
	}
	return result
}
