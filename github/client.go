package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

type CommitData struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
	Diff    string
}

type Client struct {
	client *github.Client
}

func NewClient(token string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		client: github.NewClient(tc),
	}
}

func (c *Client) GetWeeklyCommits(ctx context.Context, owner, repo string, since, until time.Time) ([]CommitData, error) {
	opts := &github.CommitsListOptions{
		Since: since,
		Until: until,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allCommits []CommitData

	for {
		commits, resp, err := c.client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("listing commits for %s/%s: %w", owner, repo, err)
		}

		for _, commit := range commits {
			if commit.Commit == nil || commit.Commit.Message == nil {
				continue
			}

			cd := CommitData{
				SHA:     commit.GetSHA(),
				Message: commit.Commit.GetMessage(),
				Date:    commit.Commit.Author.GetDate().Time,
			}

			if commit.Commit.Author != nil {
				cd.Author = commit.Commit.Author.GetName()
			}

			allCommits = append(allCommits, cd)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allCommits, nil
}

func (c *Client) GetCommitDiff(ctx context.Context, owner, repo, sha string) (string, error) {
	commit, _, err := c.client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return "", fmt.Errorf("getting commit %s: %w", sha, err)
	}

	if commit.Files == nil {
		return "", nil
	}

	var diff strings.Builder
	for _, file := range commit.Files {
		if file.Patch != nil {
			diff.WriteString(fmt.Sprintf("--- %s\n+++ %s\n%s\n\n", file.GetFilename(), file.GetFilename(), *file.Patch))
		}
	}

	return diff.String(), nil
}

func ParseRepoSlug(slug string) (owner, repo string, err error) {
	parts := strings.Split(slug, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo format %q, expected owner/repo", slug)
	}
	return parts[0], parts[1], nil
}
