// Package agent is a hand-rolled tool-use loop: call model, run tools, feed results back.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/tools"
)

type Step struct {
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Output   string          `json:"output"`
	IsError  bool            `json:"is_error"`
	Duration time.Duration   `json:"duration_ns"`
}

type Result struct {
	Answer       string `json:"answer"`
	Steps        []Step `json:"steps"`
	Turns        int    `json:"turns"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

var ErrMaxTurns = errors.New("agent exceeded max turns")

type Agent struct {
	LLM      llm.Client
	Tools    *tools.Registry
	MaxTurns int
}

const IngestPrompt = `You turn raw OCR text from a receipt into a saved structured record.
Steps: (1) work out merchant, date (YYYY-MM-DD), grand total, currency and category from the text, correcting obvious OCR noise;
(2) call find_similar_receipts; (3) if no duplicate, call save_receipt; if a duplicate exists, do NOT save.
Never invent values. If the total or date cannot be determined, do not save and explain what is missing.
Finish with one short sentence saying what happened.`

const AskPrompt = `You answer questions about the user's saved receipts. Always use tools for numbers; never guess or do arithmetic yourself.
Dates are YYYY-MM-DD. Answer concisely and state the currency if known.`

func (a *Agent) Run(ctx context.Context, system, user string) (*Result, error) {
	max := a.MaxTurns
	if max == 0 {
		max = 8
	}
	res := &Result{}
	msgs := []llm.Message{{Role: "user", Content: []llm.Block{{Type: "text", Text: user}}}}
	for res.Turns < max {
		res.Turns++
		resp, err := a.LLM.Complete(ctx, llm.Request{System: system, Messages: msgs, Tools: a.Tools.Defs()})
		if err != nil {
			return res, err
		}
		res.InputTokens += resp.Usage.InputTokens
		res.OutputTokens += resp.Usage.OutputTokens
		msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Content})

		var calls []llm.Block
		var text []string
		for _, b := range resp.Content {
			switch b.Type {
			case "tool_use":
				calls = append(calls, b)
			case "text":
				text = append(text, b.Text)
			}
		}
		if len(calls) == 0 {
			res.Answer = strings.TrimSpace(strings.Join(text, "\n"))
			return res, nil
		}
		var results []llm.Block
		for _, c := range calls {
			start := time.Now()
			out, err := a.Tools.Call(ctx, c.Name, c.Input)
			st := Step{Tool: c.Name, Input: c.Input, Output: out, Duration: time.Since(start)}
			if err != nil {
				// Tool errors go back to the model so it can self-correct.
				st.Output, st.IsError = err.Error(), true
			}
			res.Steps = append(res.Steps, st)
			results = append(results, llm.Block{Type: "tool_result", ToolUseID: c.ID, Content: st.Output, IsError: st.IsError})
		}
		msgs = append(msgs, llm.Message{Role: "user", Content: results})
	}
	return res, fmt.Errorf("%w (%d)", ErrMaxTurns, max)
}
