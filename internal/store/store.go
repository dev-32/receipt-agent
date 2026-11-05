// Package store holds receipts. The Store interface lets Postgres replace the in-memory impl.
package store

import (
	"fmt"
	"sort"
	"sync"
)

type Item struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
}

type Receipt struct {
	ID       string  `json:"id"`
	Merchant string  `json:"merchant"`
	Date     string  `json:"date"` // YYYY-MM-DD
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
	Category string  `json:"category"`
	Items    []Item  `json:"items,omitempty"`
}

type Filter struct {
	Category string
	Merchant string
	From, To string // inclusive YYYY-MM-DD, empty = open
}

type Store interface {
	Add(r Receipt) (Receipt, error)
	List(f Filter) ([]Receipt, error)
}

type Memory struct {
	mu   sync.Mutex
	rows []Receipt
}

func NewMemory() *Memory { return &Memory{} }

func (m *Memory) Add(r Receipt) (Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.ID = fmt.Sprintf("r%04d", len(m.rows)+1)
	m.rows = append(m.rows, r)
	return r, nil
}

func (m *Memory) List(f Filter) ([]Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Receipt
	for _, r := range m.rows {
		if f.Category != "" && r.Category != f.Category {
			continue
		}
		if f.Merchant != "" && !containsFold(r.Merchant, f.Merchant) {
			continue
		}
		if f.From != "" && r.Date < f.From {
			continue
		}
		if f.To != "" && r.Date > f.To {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	ls, lsub := lower(s), lower(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
