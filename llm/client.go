package llm

import (
	"context"
	"fmt"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type Client struct {
	sdk *openrouter.OpenRouter
	model string
}

func NewClient(apiKey, model string) *Client {
	sdk := openrouter.New(
		openrouter.WithSecurity(apiKey),
	)
	return &Client{
		sdk:   sdk,
		model: model,
	}
}

func (c *Client) Summarize(ctx context.Context, repoSlug string, commits []string) (string, error) {
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
