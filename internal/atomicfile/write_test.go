package atomicfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFailurePreservesOriginalAndPending(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "runtime.json")
	before := []byte(`{"port":18610}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("simulated replacement failure")
	if err := write(path, []byte(`{"port":18611}`), func(string, string) error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("unexpected failure: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("original changed before commit")
	}
	pending, err := filepath.Glob(filepath.Join(directory, ".runtime.json.pending-*"))
	if err != nil || len(pending) != 1 {
		t.Fatal("failed transaction file was not retained")
	}
}

func TestJSONReplaceAndEncodingFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := WriteJSON(path, map[string]int{"port": 18610}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(path, map[string]int{"port": 18611}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(before, []byte("18611")) {
		t.Fatal("replacement failed")
	}
	if err := WriteJSON(path, make(chan int)); err == nil {
		t.Fatal("unsupported JSON accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("encoding failure changed original")
	}
	pending, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config.json.pending-*"))
	if len(pending) != 0 {
		t.Fatal("successful writes left pending files")
	}
}
