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
		openrouter.WithTimeout(120*time.Second),
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
	prompt := fmt.Sprintf(`You are a tech lead writing a weekly report for the CEO.
	Summarize the following development work from the repository "%s" this week.
	Focus on:
	- Key features and improvements shipped
	- Bug fixes
	- Infrastructure or technical debt work
	- Any notable decisions or changes
	
	Keep it concise, professional, and non-technical where possible.
	Write in bullet points grouped by category.
	
	Commits and their changes:
	%s
	
	Provide a clear, executive-friendly summary.`, repoSlug, formatCommits(commits))

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
