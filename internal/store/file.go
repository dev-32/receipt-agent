package store

import (
	"encoding/json"
	"os"
	"sync"
)

// File persists receipts to a JSON file: zero cost, zero dependencies.
type File struct {
	mu   sync.Mutex
	path string
	mem  *Memory
}

func NewFile(path string) (*File, error) {
	f := &File{path: path, mem: NewMemory()}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &f.mem.rows); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return f, nil
}

func (f *File) Add(r Receipt) (Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	saved, err := f.mem.Add(r)
	if err != nil {
		return saved, err
	}
	b, _ := json.MarshalIndent(f.mem.rows, "", "  ")
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return saved, err
	}
	return saved, os.Rename(tmp, f.path)
}

func (f *File) List(fl Filter) ([]Receipt, error) { return f.mem.List(fl) }
