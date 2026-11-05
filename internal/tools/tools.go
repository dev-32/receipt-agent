// Package tools exposes the receipt store to the agent as callable tools.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/store"
)

type Tool struct {
	Def     llm.ToolDef
	Handler func(ctx context.Context, input json.RawMessage) (string, error)
}

type Registry struct{ tools map[string]Tool }

func (r *Registry) Defs() []llm.ToolDef {
	var out []llm.ToolDef
	for _, n := range []string{"find_similar_receipts", "save_receipt", "query_spending", "list_receipts"} {
		if t, ok := r.tools[n]; ok {
			out = append(out, t.Def)
		}
	}
	return out
}

func (r *Registry) Call(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	return t.Handler(ctx, input)
}

var categories = []string{"food", "groceries", "transport", "health", "shopping", "utilities", "entertainment", "other"}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func NewRegistry(s store.Store) *Registry {
	r := &Registry{tools: map[string]Tool{}}
	add := func(t Tool) { r.tools[t.Def.Name] = t }

	add(Tool{
		Def: llm.ToolDef{
			Name:        "find_similar_receipts",
			Description: "Look for already-saved receipts with the same merchant, date and total. Call this BEFORE save_receipt to avoid duplicates.",
			InputSchema: map[string]any{"type": "object",
				"properties": map[string]any{"merchant": str("merchant name"), "date": str("YYYY-MM-DD"), "total": map[string]any{"type": "number"}},
				"required":   []string{"merchant", "date", "total"}},
		},
		Handler: func(_ context.Context, in json.RawMessage) (string, error) {
			var a struct {
				Merchant, Date string
				Total          float64
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return "", err
			}
			rows, err := s.List(store.Filter{Merchant: a.Merchant, From: a.Date, To: a.Date})
			if err != nil {
				return "", err
			}
			var dups []store.Receipt
			for _, x := range rows {
				if math.Abs(x.Total-a.Total) < 0.005 {
					dups = append(dups, x)
				}
			}
			return jsonOut(map[string]any{"duplicates": dups})
		},
	})

	add(Tool{
		Def: llm.ToolDef{
			Name:        "save_receipt",
			Description: "Save a structured receipt. Only call after find_similar_receipts returned no duplicates.",
			InputSchema: map[string]any{"type": "object",
				"properties": map[string]any{
					"merchant": str("merchant name, cleaned of OCR noise"),
					"date":     str("YYYY-MM-DD"),
					"total":    map[string]any{"type": "number", "description": "grand total paid"},
					"currency": str("ISO code, e.g. INR"),
					"category": map[string]any{"type": "string", "enum": categories},
					"items": map[string]any{"type": "array", "items": map[string]any{"type": "object",
						"properties": map[string]any{"name": str("item"), "amount": map[string]any{"type": "number"}}}},
				},
				"required": []string{"merchant", "date", "total", "currency", "category"}},
		},
		Handler: func(_ context.Context, in json.RawMessage) (string, error) {
			var rc store.Receipt
			if err := json.Unmarshal(in, &rc); err != nil {
				return "", err
			}
			if rc.Merchant == "" || len(rc.Date) != 10 || rc.Total <= 0 {
				return "", fmt.Errorf("invalid receipt: need merchant, date YYYY-MM-DD and positive total")
			}
			if !validCategory(rc.Category) {
				return "", fmt.Errorf("invalid category %q, use one of %v", rc.Category, categories)
			}
			saved, err := s.Add(rc)
			if err != nil {
				return "", err
			}
			return jsonOut(saved)
		},
	})

	add(Tool{
		Def: llm.ToolDef{
			Name:        "query_spending",
			Description: "Total and count of saved receipts, optionally filtered. Use for any question about amounts spent.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"category": map[string]any{"type": "string", "enum": categories},
				"merchant": str("substring match"), "from": str("YYYY-MM-DD inclusive"), "to": str("YYYY-MM-DD inclusive")}},
		},
		Handler: func(_ context.Context, in json.RawMessage) (string, error) {
			var a struct{ Category, Merchant, From, To string }
			if err := json.Unmarshal(in, &a); err != nil {
				return "", err
			}
			rows, err := s.List(store.Filter{Category: a.Category, Merchant: a.Merchant, From: a.From, To: a.To})
			if err != nil {
				return "", err
			}
			sum := 0.0
			for _, x := range rows {
				sum += x.Total
			}
			return jsonOut(map[string]any{"count": len(rows), "total": math.Round(sum*100) / 100})
		},
	})

	add(Tool{
		Def: llm.ToolDef{
			Name:        "list_receipts",
			Description: "List saved receipts (newest first), optionally filtered.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{
				"category": map[string]any{"type": "string", "enum": categories},
				"merchant": str("substring match"), "from": str("YYYY-MM-DD"), "to": str("YYYY-MM-DD"),
				"limit": map[string]any{"type": "integer"}}},
		},
		Handler: func(_ context.Context, in json.RawMessage) (string, error) {
			var a struct {
				Category, Merchant, From, To string
				Limit                        int
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return "", err
			}
			rows, err := s.List(store.Filter{Category: a.Category, Merchant: a.Merchant, From: a.From, To: a.To})
			if err != nil {
				return "", err
			}
			if a.Limit <= 0 || a.Limit > 50 {
				a.Limit = 20
			}
			if len(rows) > a.Limit {
				rows = rows[:a.Limit]
			}
			return jsonOut(rows)
		},
	})
	return r
}

func validCategory(c string) bool {
	for _, x := range categories {
		if x == c {
			return true
		}
	}
	return false
}

func jsonOut(v any) (string, error) { b, err := json.Marshal(v); return string(b), err }
