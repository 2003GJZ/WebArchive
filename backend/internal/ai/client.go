package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

type TagInput struct {
	Title   string
	URL     string
	Content string
	Excerpt string
}

type TagResult struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Path     []string `json:"path"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewClient(baseURL, apiKey, model string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

func (c *Client) Tag(ctx context.Context, input TagInput) (TagResult, error) {
	if !c.Enabled() {
		return TagResult{}, errors.New("llm not configured")
	}

	content := strings.TrimSpace(input.Content)
	if len(content) > 6000 {
		content = content[:6000]
	}

	system := "You are a taxonomy assistant. Return strict JSON only."
	user := fmt.Sprintf(
		"Generate a compact knowledge classification for the following content.\n"+
			"Return JSON with fields: category (string), tags (array of short strings), path (array of strings from high-level to low-level).\n"+
			"Title: %s\nURL: %s\nExcerpt: %s\nContent: %s",
		input.Title, input.URL, input.Excerpt, content,
	)

	raw, err := c.ChatJSON(ctx, system, user, 0.2)
	if err != nil {
		return TagResult{}, err
	}
	raw = extractJSON(raw)
	if raw == "" {
		return TagResult{}, errors.New("llm invalid json")
	}

	var out TagResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return TagResult{}, err
	}
	out.Tags = normalizeList(out.Tags)
	out.Path = normalizeList(out.Path)
	return out, nil
}

func (c *Client) ChatJSON(ctx context.Context, system, user string, temperature float64) (string, error) {
	if !c.Enabled() {
		return "", errors.New("llm not configured")
	}
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: temperature,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("llm error: %s", strings.TrimSpace(string(body)))
	}

	var res chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", errors.New("llm empty response")
	}
	return strings.TrimSpace(res.Choices[0].Message.Content), nil
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(c.BaseURL, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func normalizeList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
