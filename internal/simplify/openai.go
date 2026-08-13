package simplify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openAIChatReq struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type openAIChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func openAIBaseURL(base string) string {
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func (c *Client) chatOpenAI(ctx context.Context, userPrompt string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 800
	}
	body, _ := json.Marshal(openAIChatReq{
		Model:       c.Model,
		Messages:    []chatMessage{{Role: "user", Content: userPrompt}},
		Temperature: 0.2,
		MaxTokens:   maxTokens,
		Stream:      false,
	})
	url := openAIBaseURL(c.BaseURL) + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("freebuff chat: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		msg := string(raw)
		if strings.Contains(msg, "free_mode_invalid_agent_hierarchy") {
			return "", fmt.Errorf("freebuff: use a root model (minimax, kimi, deepseek) — not Gemini subagents: %s", msg)
		}
		if strings.Contains(msg, "model_not_found") || strings.Contains(msg, "unsupported model") {
			return "", fmt.Errorf("freebuff: model %q not on proxy — refresh Freebuff2API and pick a listed root model: %s", c.Model, msg)
		}
		return "", fmt.Errorf("freebuff %s: %s", res.Status, msg)
	}
	var out openAIChatResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("freebuff: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("%s returned no choices", c.Model)
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("%s returned empty content", c.Model)
	}
	return text, nil
}
