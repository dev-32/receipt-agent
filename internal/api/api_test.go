package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev-32/receipt-agent/internal/agent"
	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/store"
	"github.com/dev-32/receipt-agent/internal/tools"
)

type fake struct{ n int }

func (f *fake) Complete(_ context.Context, _ llm.Request) (*llm.Response, error) {
	f.n++
	if f.n == 1 {
		return &llm.Response{Content: []llm.Block{{Type: "tool_use", ID: "1", Name: "save_receipt",
			Input: json.RawMessage(`{"merchant":"Uber","date":"2026-10-18","total":223.13,"currency":"INR","category":"transport"}`)}}}, nil
	}
	return &llm.Response{Content: []llm.Block{{Type: "text", Text: "Saved."}}}, nil
}

func newServer(key string) (*httptest.Server, *store.Memory) {
	st := store.NewMemory()
	s := &Server{Agent: &agent.Agent{LLM: &fake{}, Tools: tools.NewRegistry(st)}, Store: st, APIKey: key, Origin: "*"}
	return httptest.NewServer(s.Handler()), st
}

func TestIngestAndList(t *testing.T) {
	srv, st := newServer("")
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/v1/receipts", "application/json", strings.NewReader(`{"ocr_text":"Uber 223.13"}`))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status err=%v resp=%v", err, resp)
	}
	if rows, _ := st.List(store.Filter{}); len(rows) != 1 {
		t.Fatalf("not saved: %+v", rows)
	}
	resp, _ = http.Get(srv.URL + "/v1/receipts?category=transport")
	var rows []store.Receipt
	json.NewDecoder(resp.Body).Decode(&rows)
	if len(rows) != 1 || rows[0].Merchant != "Uber" {
		t.Fatalf("list: %+v", rows)
	}
}

func TestAuthAndValidation(t *testing.T) {
	srv, _ := newServer("secret")
	defer srv.Close()
	resp, _ := http.Post(srv.URL+"/v1/ask", "application/json", strings.NewReader(`{"question":"x"}`))
	if resp.StatusCode != 401 {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/v1/ask", strings.NewReader(`{}`))
	req.Header.Set("X-API-Key", "secret")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != 400 {
		t.Fatalf("want 400 got %d", resp.StatusCode)
	}
	resp, _ = http.Get(srv.URL + "/healthz")
	if resp.StatusCode != 200 {
		t.Fatal("healthz should be open")
	}
}
