package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitialPasswordFileAndClear(t *testing.T) {
	directory := t.TempDir()
	manager, generated, err := Open(directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if generated == "" {
		t.Fatal("expected generated initial password")
	}
	path, err := WriteInitialPassword(directory, generated)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != generated+"\n" {
		t.Fatalf("initial password file = %q", string(data))
	}
	if filepath.Dir(path) != directory {
		t.Fatalf("initial password file outside data directory: %s", path)
	}
	manager.clearInitialPassword()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("initial password file was not removed: %v", err)
	}
}
