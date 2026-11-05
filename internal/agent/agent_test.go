package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/store"
	"github.com/dev-32/receipt-agent/internal/tools"
)

// script returns canned responses in order and records the requests it saw.
type script struct {
	resps []*llm.Response
	seen  []llm.Request
}

func (s *script) Complete(_ context.Context, r llm.Request) (*llm.Response, error) {
	s.seen = append(s.seen, r)
	i := len(s.seen) - 1
	if i >= len(s.resps) {
		i = len(s.resps) - 1
	}
	return s.resps[i], nil
}

func call(id, name, in string) *llm.Response {
	return &llm.Response{StopReason: "tool_use", Content: []llm.Block{{Type: "tool_use", ID: id, Name: name, Input: json.RawMessage(in)}}}
}
func say(t string) *llm.Response {
	return &llm.Response{StopReason: "end_turn", Content: []llm.Block{{Type: "text", Text: t}}}
}

const swiggy = `{"merchant":"Swiggy","date":"2026-10-12","total":450,"currency":"INR","category":"food"}`

func TestIngestSavesReceipt(t *testing.T) {
	st := store.NewMemory()
	s := &script{resps: []*llm.Response{
		call("1", "find_similar_receipts", `{"merchant":"Swiggy","date":"2026-10-12","total":450}`),
		call("2", "save_receipt", swiggy),
		say("Saved."),
	}}
	a := &Agent{LLM: s, Tools: tools.NewRegistry(st)}
	res, err := a.Run(context.Background(), IngestPrompt, "ocr")
	if err != nil || res.Answer != "Saved." || len(res.Steps) != 2 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	rows, _ := st.List(store.Filter{})
	if len(rows) != 1 || rows[0].Merchant != "Swiggy" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestDuplicateDetected(t *testing.T) {
	st := store.NewMemory()
	st.Add(store.Receipt{Merchant: "Swiggy", Date: "2026-10-12", Total: 450, Category: "food"})
	s := &script{resps: []*llm.Response{
		call("1", "find_similar_receipts", `{"merchant":"swiggy","date":"2026-10-12","total":450}`),
		say("Duplicate, not saved."),
	}}
	a := &Agent{LLM: s, Tools: tools.NewRegistry(st)}
	res, _ := a.Run(context.Background(), IngestPrompt, "ocr")
	if !strings.Contains(res.Steps[0].Output, "r0001") {
		t.Fatalf("duplicate not found: %s", res.Steps[0].Output)
	}
}

func TestToolErrorFedBackToModel(t *testing.T) {
	st := store.NewMemory()
	s := &script{resps: []*llm.Response{
		call("1", "save_receipt", `{"merchant":"X","date":"2026-10-12","total":10,"currency":"INR","category":"bogus"}`),
		say("done"),
	}}
	a := &Agent{LLM: s, Tools: tools.NewRegistry(st)}
	res, _ := a.Run(context.Background(), "", "x")
	if !res.Steps[0].IsError {
		t.Fatal("expected error step")
	}
	last := s.seen[1].Messages[len(s.seen[1].Messages)-1].Content[0]
	if last.Type != "tool_result" || !last.IsError {
		t.Fatalf("error not returned to model: %+v", last)
	}
}

func TestMaxTurns(t *testing.T) {
	s := &script{resps: []*llm.Response{call("1", "list_receipts", `{}`)}}
	a := &Agent{LLM: s, Tools: tools.NewRegistry(store.NewMemory()), MaxTurns: 3}
	_, err := a.Run(context.Background(), "", "x")
	if !errors.Is(err, ErrMaxTurns) {
		t.Fatalf("err=%v", err)
	}
}

func TestQuerySpending(t *testing.T) {
	st := store.NewMemory()
	st.Add(store.Receipt{Merchant: "A", Date: "2026-10-01", Total: 100.5, Category: "food"})
	st.Add(store.Receipt{Merchant: "B", Date: "2026-10-20", Total: 50, Category: "food"})
	st.Add(store.Receipt{Merchant: "C", Date: "2026-09-20", Total: 999, Category: "food"})
	out, err := tools.NewRegistry(st).Call(context.Background(), "query_spending", json.RawMessage(`{"category":"food","from":"2026-10-01","to":"2026-10-31"}`))
	if err != nil || out != `{"count":2,"total":150.5}` {
		t.Fatalf("out=%s err=%v", out, err)
	}
}
