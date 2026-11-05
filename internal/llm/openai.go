package llm

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

// OpenAICompat talks to any OpenAI-style /chat/completions endpoint with tool calling:
// Groq (free tier), Google Gemini (free tier), Ollama (local, free), OpenRouter free models.
// It converts to/from the Anthropic-style blocks the agent loop uses.
type OpenAICompat struct {
	BaseURL string
	APIKey  string // may be empty (Ollama)
	Model   string
	HTTP    *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model,
		HTTP: &http.Client{Timeout: 120 * time.Second}}
}

type oaMsg struct {
	Role       string       `json:"role"`
	Content    *string      `json:"content"`
	ToolCalls  []oaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}
type oaToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func sp(s string) *string { return &s }

func toOpenAI(req Request) (msgs []oaMsg, tools []map[string]any) {
	if req.System != "" {
		msgs = append(msgs, oaMsg{Role: "system", Content: sp(req.System)})
	}
	for _, m := range req.Messages {
		var text []string
		var calls []oaToolCall
		var toolMsgs []oaMsg
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				text = append(text, b.Text)
			case "tool_use":
				tc := oaToolCall{ID: b.ID, Type: "function"}
				tc.Function.Name = b.Name
				tc.Function.Arguments = string(b.Input)
				if tc.Function.Arguments == "" {
					tc.Function.Arguments = "{}"
				}
				calls = append(calls, tc)
			case "tool_result":
				toolMsgs = append(toolMsgs, oaMsg{Role: "tool", ToolCallID: b.ToolUseID, Content: sp(b.Content)})
			}
		}
		if m.Role == "assistant" {
			am := oaMsg{Role: "assistant", ToolCalls: calls}
			if len(text) > 0 {
				am.Content = sp(strings.Join(text, "\n"))
			}
			msgs = append(msgs, am)
		} else {
			msgs = append(msgs, toolMsgs...)
			if len(text) > 0 {
				msgs = append(msgs, oaMsg{Role: "user", Content: sp(strings.Join(text, "\n"))})
			}
		}
	}
	for _, t := range req.Tools {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name": t.Name, "description": t.Description, "parameters": t.InputSchema}})
	}
	return
}

func (o *OpenAICompat) Complete(ctx context.Context, req Request) (*Response, error) {
	msgs, tools := toOpenAI(req)
	body := map[string]any{"model": o.Model, "messages": msgs, "temperature": 0}
	if len(tools) > 0 {
		body["tools"] = tools
	}
	buf, _ := json.Marshal(body)

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 { // free tiers rate-limit hard: back off longer
			select {
			case <-time.After(time.Duration(attempt*attempt*3) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		hr, _ := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/chat/completions", bytes.NewReader(buf))
		hr.Header.Set("content-type", "application/json")
		if o.APIKey != "" {
			hr.Header.Set("authorization", "Bearer "+o.APIKey)
		}
		resp, err := o.HTTP.Do(hr)
		if err != nil {
			lastErr = err
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("llm %d: %s", resp.StatusCode, data)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("llm %d: %s", resp.StatusCode, data)
		}
		return fromOpenAI(data)
	}
	return nil, lastErr
}

func fromOpenAI(data []byte) (*Response, error) {
	var r struct {
		Choices []struct {
			Message struct {
				Content   *string      `json:"content"`
				ToolCalls []oaToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if len(r.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices: %s", data)
	}
	c := r.Choices[0]
	out := &Response{StopReason: c.FinishReason, Usage: Usage{InputTokens: r.Usage.Prompt, OutputTokens: r.Usage.Completion}}
	if c.Message.Content != nil && *c.Message.Content != "" {
		out.Content = append(out.Content, Block{Type: "text", Text: *c.Message.Content})
	}
	for i, tc := range c.Message.ToolCalls {
		id := tc.ID
		if id == "" { // some free providers omit ids
			id = fmt.Sprintf("call_%d", i)
		}
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out.Content = append(out.Content, Block{Type: "tool_use", ID: id, Name: tc.Function.Name, Input: json.RawMessage(args)})
	}
	return out, nil
}
