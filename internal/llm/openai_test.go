package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatRoundTrip(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		if r.Header.Get("authorization") != "Bearer k" {
			t.Error("missing bearer")
		}
		w.Write([]byte(`{"choices":[{"finish_reason":"tool_calls","message":{"content":null,"tool_calls":[{"function":{"name":"list_receipts","arguments":""}}]}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`))
	}))
	defer srv.Close()

	c := NewOpenAICompat(srv.URL, "k", "m")
	resp, err := c.Complete(context.Background(), Request{
		System: "sys",
		Tools:  []ToolDef{{Name: "list_receipts", Description: "d", InputSchema: map[string]any{"type": "object"}}},
		Messages: []Message{
			{Role: "user", Content: []Block{{Type: "text", Text: "hi"}}},
			{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "a", Name: "x", Input: json.RawMessage(`{"q":1}`)}}},
			{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "a", Content: "ok"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Content) != 1 || resp.Content[0].Name != "list_receipts" || resp.Content[0].ID != "call_0" || string(resp.Content[0].Input) != "{}" {
		t.Fatalf("bad response conversion: %+v", resp.Content)
	}
	if resp.Usage.InputTokens != 7 {
		t.Fatal("usage lost")
	}
	msgs := got["messages"].([]any)
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	want := []string{"system", "user", "assistant", "tool"}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles=%v", roles)
		}
	}
}
