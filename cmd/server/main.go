package main

import (
	"log"
	"net/http"
	"os"

	"github.com/dev-32/receipt-agent/internal/agent"
	"github.com/dev-32/receipt-agent/internal/api"
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
	srv := &api.Server{
		Agent:  &agent.Agent{LLM: client, Tools: tools.NewRegistry(st)},
		Store:  st,
		APIKey: os.Getenv("API_KEY"),
		Origin: os.Getenv("CORS_ORIGIN"),
	}
	if srv.APIKey == "" {
		log.Println("warning: API_KEY not set, /v1/* is open")
	}
	addr := ":" + envOr("PORT", "8080")
	log.Printf("listening on %s (model %s)", addr, model)
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
