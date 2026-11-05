// Eval harness: runs the ingest agent over labeled OCR texts and scores each field.
// Usage: ANTHROPIC_API_KEY=... go run ./cmd/eval [-file testdata/receipts.json]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/dev-32/receipt-agent/internal/agent"
	"github.com/dev-32/receipt-agent/internal/llm"
	"github.com/dev-32/receipt-agent/internal/store"
	"github.com/dev-32/receipt-agent/internal/tools"
)

type Case struct {
	Name     string `json:"name"`
	OCRText  string `json:"ocr_text"`
	Expected struct {
		Merchant   string  `json:"merchant"`
		Date       string  `json:"date"`
		Total      float64 `json:"total"`
		Category   string  `json:"category"`
		ShouldSave *bool   `json:"should_save"`
	} `json:"expected"`
}

func main() {
	file := flag.String("file", "testdata/receipts.json", "dataset")
	sleep := flag.Duration("sleep", 0, "pause between cases (free tiers rate-limit; try 3s)")
	out := flag.String("out", "", "write a markdown results file")
	flag.Parse()
	client, model, err := llm.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		panic(err)
	}
	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		panic(err)
	}

	var pass, tokIn, tokOut int
	var report strings.Builder
	var totalDur time.Duration
	for i, c := range cases {
		if i > 0 && *sleep > 0 {
			time.Sleep(*sleep)
		}
		st := store.NewMemory()
		ag := &agent.Agent{LLM: client, Tools: tools.NewRegistry(st)}
		start := time.Now()
		res, err := ag.Run(context.Background(), agent.IngestPrompt, c.OCRText)
		totalDur += time.Since(start)
		if err != nil {
			fmt.Printf("FAIL %-20s error: %v\n", c.Name, err)
			fmt.Fprintf(&report, "| %s | FAIL | error: %v |\n", c.Name, err)
			continue
		}
		tokIn += res.InputTokens
		tokOut += res.OutputTokens
		rows, _ := st.List(store.Filter{})
		var problems []string
		if c.Expected.ShouldSave != nil && !*c.Expected.ShouldSave {
			if len(rows) != 0 {
				problems = append(problems, "saved a receipt that should have been rejected")
			}
		} else if len(rows) != 1 {
			problems = append(problems, fmt.Sprintf("expected 1 saved receipt, got %d", len(rows)))
		} else {
			r := rows[0]
			if !strings.EqualFold(r.Merchant, c.Expected.Merchant) {
				problems = append(problems, fmt.Sprintf("merchant %q != %q", r.Merchant, c.Expected.Merchant))
			}
			if r.Date != c.Expected.Date {
				problems = append(problems, fmt.Sprintf("date %s != %s", r.Date, c.Expected.Date))
			}
			if math.Abs(r.Total-c.Expected.Total) > 0.005 {
				problems = append(problems, fmt.Sprintf("total %.2f != %.2f", r.Total, c.Expected.Total))
			}
			if r.Category != c.Expected.Category {
				problems = append(problems, fmt.Sprintf("category %s != %s", r.Category, c.Expected.Category))
			}
		}
		if len(problems) == 0 {
			pass++
			fmt.Printf("PASS %-20s turns=%d\n", c.Name, res.Turns)
			fmt.Fprintf(&report, "| %s | PASS | |\n", c.Name)
		} else {
			fmt.Printf("FAIL %-20s %s\n", c.Name, strings.Join(problems, "; "))
			fmt.Fprintf(&report, "| %s | FAIL | %s |\n", c.Name, strings.Join(problems, "; "))
		}
	}
	fmt.Printf("\n%d/%d passed | model=%s | tokens in=%d out=%d | avg latency %s\n",
		pass, len(cases), model, tokIn, tokOut, (totalDur / time.Duration(len(cases))).Round(time.Millisecond))
	if *out != "" {
		head := fmt.Sprintf("# Eval results\n\nModel: `%s`  \nPassed: **%d/%d**  \nTokens in/out: %d/%d\n\n| case | result | notes |\n|---|---|---|\n", model, pass, len(cases), tokIn, tokOut)
		os.WriteFile(*out, []byte(head+report.String()), 0o644)
	}
	if pass != len(cases) {
		os.Exit(1)
	}
}
