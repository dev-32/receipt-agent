package store

import (
	"path/filepath"
	"testing"
)

func TestFilePersists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.json")
	f, _ := NewFile(p)
	f.Add(Receipt{Merchant: "A", Date: "2026-10-01", Total: 5, Category: "food"})
	g, err := NewFile(p)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := g.List(Filter{})
	if len(rows) != 1 || rows[0].ID != "r0001" {
		t.Fatalf("%+v", rows)
	}
	g.Add(Receipt{Merchant: "B", Date: "2026-10-02", Total: 6, Category: "food"})
	if rows, _ := g.List(Filter{}); rows[0].ID != "r0002" && rows[1].ID != "r0002" {
		t.Fatalf("ids not continuing: %+v", rows)
	}
}
