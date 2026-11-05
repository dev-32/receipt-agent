// Package llm defines a minimal provider interface plus an Anthropic Messages API client.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type Message struct {
	Role    string  `json:"role"`
	Content []Block `json:"content"`
}

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDef
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Response struct {
	Content    []Block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Usage      Usage   `json:"usage"`
}

// Client is the only thing the agent loop depends on, so tests can script it.
type Client interface {
	Complete(ctx context.Context, req Request) (*Response, error)
}

type Anthropic struct {
	APIKey    string
	Model     string
	MaxTokens int
	BaseURL   string
	HTTP      *http.Client
}

func NewAnthropic(apiKey, model string) *Anthropic {
	return &Anthropic{APIKey: apiKey, Model: model, MaxTokens: 2048,
		BaseURL: "https://api.anthropic.com", HTTP: &http.Client{Timeout: 60 * time.Second}}
}

func (a *Anthropic) Complete(ctx context.Context, req Request) (*Response, error) {
	body := map[string]any{
		"model":      a.Model,
		"max_tokens": a.MaxTokens,
		"system":     req.System,
		"messages":   req.Messages,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		hr, _ := http.NewRequestWithContext(ctx, "POST", a.BaseURL+"/v1/messages", bytes.NewReader(buf))
		hr.Header.Set("x-api-key", a.APIKey)
		hr.Header.Set("anthropic-version", "2023-06-01")
		hr.Header.Set("content-type", "application/json")
		resp, err := a.HTTP.Do(hr)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("anthropic %d: %s", resp.StatusCode, data)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, data)
		}
		var out Response
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	return nil, lastErr
}
