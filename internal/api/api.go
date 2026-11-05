// Package api is the HTTP layer: ingest OCR text, ask questions, list receipts.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/dev-32/receipt-agent/internal/agent"
	"github.com/dev-32/receipt-agent/internal/store"
)

type Server struct {
	Agent   *agent.Agent
	Store   store.Store
	APIKey  string        // if set, /v1/* requires header X-API-Key
	Origin  string        // CORS allow-origin, "" = none
	Timeout time.Duration // per-request agent timeout
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("POST /v1/receipts", s.auth(s.agentHandler(agent.IngestPrompt, "ocr_text")))
	mux.HandleFunc("POST /v1/ask", s.auth(s.agentHandler(agent.AskPrompt, "question")))
	mux.HandleFunc("GET /v1/receipts", s.auth(s.list))
	return s.cors(mux)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.Origin)
			w.Header().Set("Access-Control-Allow-Headers", "content-type, x-api-key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.APIKey != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(s.APIKey)) != 1 {
			writeErr(w, 401, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) agentHandler(system, field string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in[field] == "" {
			writeErr(w, 400, "need "+field)
			return
		}
		to := s.Timeout
		if to == 0 {
			to = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), to)
		defer cancel()
		res, err := s.Agent.Run(ctx, system, in[field])
		if err != nil {
			log.Printf("agent error: %v", err)
			writeErr(w, 502, "agent failed")
			return
		}
		writeJSON(w, 200, res)
	}
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, err := s.Store.List(store.Filter{Category: q.Get("category"), Merchant: q.Get("merchant"), From: q.Get("from"), To: q.Get("to")})
	if err != nil {
		writeErr(w, 500, "store error")
		return
	}
	if rows == nil {
		rows = []store.Receipt{}
	}
	writeJSON(w, 200, rows)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
