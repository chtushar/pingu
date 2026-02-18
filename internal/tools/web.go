package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	bravesearch "github.com/cnosuke/go-brave-search"
)

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

type Web struct {
	brave *bravesearch.Client
}

func NewWeb(braveAPIKey string) *Web {
	client, _ := bravesearch.NewClient(braveAPIKey)
	return &Web{brave: client}
}

func (w *Web) Name() string { return "web" }
func (w *Web) Description() string {
	return "Search the web or fetch content from a URL"
}

func (w *Web) InputSchema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"search", "fetch"},
				"description": "Operation: search the web or fetch a URL",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Search query (required for search action)",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "URL to fetch (required for fetch action)",
			},
			"count": map[string]any{
				"type":        "number",
				"description": "Number of search results to return (default 5, max 20)",
			},
		},
		"required":             []string{"action", "query", "url", "count"},
		"additionalProperties": false,
	}
}

func (w *Web) Execute(ctx context.Context, input string) (string, error) {
	var args struct {
		Action string `json:"action"`
		Query  string `json:"query"`
		URL    string `json:"url"`
		Count  int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return "", fmt.Errorf("parsing web input: %w", err)
	}

	switch args.Action {
	case "search":
		return w.search(ctx, args.Query, args.Count)
	case "fetch":
		return w.fetch(ctx, args.URL)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func (w *Web) search(ctx context.Context, query string, count int) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required for search action")
	}
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}

	slog.Debug("web: searching", "query", query, "count", count)

	resp, err := w.brave.WebSearch(ctx, query, &bravesearch.WebSearchParams{
		Count: count,
	})
	if err != nil {
		return "", fmt.Errorf("brave search: %w", err)
	}

	results := resp.GetWebResults()
	if len(results) == 0 {
		return "No results found.", nil
	}

	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		fmt.Fprintf(&b, "%s\n%s\n%s", r.Title, r.URL, r.Description)
	}

	slog.Debug("web: search done", "query", query, "results", len(results))
	return truncate([]byte(b.String())), nil
}

func (w *Web) fetch(ctx context.Context, url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("url is required for fetch action")
	}

	slog.Debug("web: fetching", "url", url)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "pingu/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	const maxBody = 100 * 1024 // 100KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	text := htmlTagRe.ReplaceAllString(string(body), "")
	// Collapse whitespace runs into single spaces/newlines.
	text = strings.Join(strings.Fields(text), " ")

	slog.Debug("web: fetch done", "url", url, "bytes", len(text))
	return truncate([]byte(text)), nil
}
