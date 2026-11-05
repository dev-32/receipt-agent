# receipt-agent

A Go service that turns noisy on-device OCR text (from my Flutter Doc Scanner app) into structured
receipts and answers spending questions, using a hand-written tool-use agent loop on an LLM (free providers supported).

## Why
OCR text is messy (`B1GBASKET`, `410.0O`). Regexes break; a model fixes the noise, but an unconstrained
model also invents totals. So the model only acts through validated tools, checks for duplicates before saving,
and never does arithmetic itself (sums come from `query_spending`).

## Design
- `internal/agent`: custom loop (max turns, tool errors fed back to the model so it can self-correct, token accounting, per-step trace)
- `internal/tools`: `find_similar_receipts`, `save_receipt`, `query_spending`, `list_receipts` (input validation lives here, not in the prompt)
- `internal/llm`: small `Client` interface + Anthropic Messages API client with retry; tests use a scripted fake
- `internal/store`: `Store` interface (in-memory and JSON file; Postgres/Supabase can slot in later)
- `cmd/eval`: labeled OCR cases scored field by field (merchant/date/total/category + refusal on unreadable input)

## Run it for free ($0, no card)
The LLM layer is provider-agnostic. Pick one free option:

| Provider | Cost | Setup |
|---|---|---|
| Groq | free tier, no card | free key at console.groq.com, `LLM_PROVIDER=groq` (default) |
| Gemini | free tier, no card | free key at aistudio.google.com, `LLM_PROVIDER=gemini` |
| Ollama | fully local, no key, no limits | `ollama pull qwen2.5:7b`, `LLM_PROVIDER=ollama` |
| OpenRouter | `:free` models | `LLM_PROVIDER=openrouter` |

Free-tier model names change; override with `LLM_MODEL=...`. Paid Claude still works: `LLM_PROVIDER=anthropic`.

    go test ./...                                   # no key needed
    LLM_API_KEY=... go run ./cmd/eval               # accuracy, tokens, latency
    LLM_API_KEY=... go run ./cmd/server             # data persists in receipts.json
    curl -XPOST localhost:8080/v1/receipts -d '{"ocr_text":"SWIGGY ... Total 450"}'
    curl -XPOST localhost:8080/v1/ask -d '{"question":"How much did I spend on food in October?"}'

Storage is a JSON file (no database bill). Hosting options at $0: run locally and expose with a free tunnel
(cloudflared), or deploy the Dockerfile to a free tier (Render, Hugging Face Spaces). Note free hosts have
ephemeral disks, so the file store resets on redeploy; fine for a demo. Distribute the Flutter app as an APK
via GitHub Releases (Play Store needs a one-time $25 fee, so skip it).

## API
| Route | What it does |
|---|---|
| `POST /v1/receipts` `{"ocr_text": "..."}` | agent cleans, de-duplicates and saves a receipt |
| `POST /v1/ask` `{"question": "..."}` | agent answers using the query tools |
| `GET /v1/receipts?category=food&from=2026-10-01` | list saved receipts |
| `GET /healthz` | health check |

Set `API_KEY` to require an `X-API-Key` header, and `CORS_ORIGIN` for browser clients.
`flutter_client/receipt_agent_client.dart` is a small Dart client for the Doc Scanner app.

## Eval
31 labeled cases (clean, noisy OCR, multiple date formats, and inputs that must be rejected).

    LLM_API_KEY=... go run ./cmd/eval -sleep 3s -out RESULTS.md

`-sleep` avoids free-tier rate limits. CI runs `go vet` and `go test -race` on every push.

## Deploy for free
`render.yaml` is a Render blueprint (free plan). Set `LLM_API_KEY` and `API_KEY` in the dashboard.
Free hosts have an ephemeral disk, so saved receipts reset on redeploy.

## Status
- [x] agent loop, tools, unit tests, CI
- [x] free LLM providers (Groq / Gemini / Ollama), file store
- [x] HTTP API with auth, CORS and tests; Dockerfile and Render blueprint
- [x] 31-case eval set, markdown results output
- [x] Dart client for Flutter
- [ ] publish eval results in RESULTS.md (run the eval with a real key)
- [ ] wire the Dart client into Doc Scanner
- [ ] public demo URL
