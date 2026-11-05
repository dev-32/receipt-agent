package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dev-32/receipt-agent/internal/agent"
	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/store"
	"github.com/dev-32/receipt-agent/internal/tools"
)

func main() {
	client, model, err := llm.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.NewFile(envOr("DATA_FILE", "receipts.json"))
	if err != nil {
		log.Fatal(err)
	}
	ag := &agent.Agent{LLM: client, Tools: tools.NewRegistry(st)}

	mux := http.NewServeMux()
	handle := func(system, field string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var in map[string]string
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in[field] == "" {
				http.Error(w, `{"error":"need `+field+`"}`, 400)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
			defer cancel()
			res, err := ag.Run(ctx, system, in[field])
			if err != nil {
				log.Printf("agent error: %v", err)
				http.Error(w, `{"error":"agent failed"}`, 502)
				return
			}
			w.Header().Set("content-type", "application/json")
			json.NewEncoder(w).Encode(res)
		}
	}
	mux.HandleFunc("POST /v1/receipts", handle(agent.IngestPrompt, "ocr_text"))
	mux.HandleFunc("POST /v1/ask", handle(agent.AskPrompt, "question"))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	addr := ":" + envOr("PORT", "8080")
	log.Printf("listening on %s (model %s)", addr, model)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
