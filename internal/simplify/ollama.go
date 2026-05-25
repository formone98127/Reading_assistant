package simplify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model     string         `json:"model"`
	Messages  []chatMessage  `json:"messages"`
	Stream    bool           `json:"stream"`
	Think     bool           `json:"think"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

type chatResp struct {
	Message struct {
		Content   string `json:"content"`
		Thinking  string `json:"thinking"`
	} `json:"message"`
	Error      string `json:"error"`
	DoneReason string `json:"done_reason"`
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 5 * time.Minute}
	}
	return c.HTTP
}

func batchOptions() map[string]any {
	return map[string]any{
		"temperature": 0.2,
		"num_predict": 800,
	}
}

func singleOptions() map[string]any {
	return map[string]any{
		"temperature": 0.2,
		"num_predict": 250,
	}
}

func (c *Client) chat(ctx context.Context, userPrompt string, opts map[string]any) (string, error) {
	if opts == nil {
		opts = batchOptions()
	}
	body, _ := json.Marshal(chatReq{
		Model:     c.Model,
		Messages:  []chatMessage{{Role: "user", Content: userPrompt}},
		Stream:    false,
		Think:     false, // required for gemma4 — thinking burns num_predict and leaves content empty
		KeepAlive: "30m",
		Options:   opts,
	})
	url := strings.TrimSuffix(c.BaseURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama chat: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama %s: %s", res.Status, string(raw))
	}
	var out chatResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama: %s", out.Error)
	}
	text := strings.TrimSpace(out.Message.Content)
	if text == "" && strings.TrimSpace(out.Message.Thinking) != "" {
		// Last resort: model put answer only in thinking trace
		text = strings.TrimSpace(out.Message.Thinking)
	}
	if text == "" {
		return "", fmt.Errorf("%s returned no text (done_reason=%s) — restart Ollama and try again", c.Model, out.DoneReason)
	}
	return text, nil
}

// SimplifyAllLevels returns up to 3 easier versions in one model call.
func (c *Client) SimplifyAllLevels(ctx context.Context, original string) ([]string, string, error) {
	raw, err := c.chat(ctx, BatchPrompt(original), batchOptions())
	if err != nil {
		return c.simplifySequential(ctx, original, err)
	}
	levels := LevelsFromResponse(raw, original)
	if len(levels) > 0 {
		return levels, raw, nil
	}

	big := batchOptions()
	big["num_predict"] = 1200
	raw2, err2 := c.chat(ctx, BatchPrompt(original), big)
	if err2 == nil {
		levels = LevelsFromResponse(raw2, original)
		if len(levels) > 0 {
			return levels, raw2, nil
		}
		raw = raw2
	}

	return c.simplifySequential(ctx, original, fmt.Errorf("could not parse %s output: %q", c.Model, truncate(raw, 300)))
}

func (c *Client) simplifySequential(ctx context.Context, original string, batchErr error) ([]string, string, error) {
	var levels []string
	var raws []string
	prev := ""
	for level := 1; level <= 3; level++ {
		if level == 1 {
			prev = original
		}
		raw, err := c.chat(ctx, SinglePrompt(original, level, prev), singleOptions())
		if err != nil {
			if len(levels) > 0 {
				return levels, strings.Join(raws, "\n---\n"), nil
			}
			if batchErr != nil {
				return nil, "", fmt.Errorf("%v; fallback failed: %w", batchErr, err)
			}
			return nil, "", err
		}
		line := firstUsefulLine(raw, original)
		if line == "" {
			line = strings.TrimSpace(raw)
		}
		if line == "" {
			continue
		}
		levels = append(levels, line)
		raws = append(raws, raw)
		prev = line
	}
	if len(levels) == 0 {
		if batchErr != nil {
			return nil, "", batchErr
		}
		return nil, "", fmt.Errorf("no simplifications from %s", c.Model)
	}
	return levels, strings.Join(raws, "\n---\n"), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *Client) Warm(ctx context.Context) error {
	_, err := c.chat(ctx, "Reply with exactly: ok", map[string]any{"num_predict": 16})
	return err
}

// TranslateChinese returns a Traditional Chinese (繁體) translation of one sentence.
func (c *Client) TranslateChinese(ctx context.Context, original string) (string, error) {
	raw, err := c.chat(ctx, ChinesePrompt(original), singleOptions())
	if err != nil {
		return "", err
	}
	line := firstUsefulLine(raw, original)
	if line == "" {
		line = strings.TrimSpace(raw)
	}
	if line == "" {
		return "", fmt.Errorf("%s returned no Chinese translation", c.Model)
	}
	return line, nil
}
