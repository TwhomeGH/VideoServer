package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestAddWatchTreeSkipsSystemDirectories(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"Videos", "Windows", "node_modules", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	added, err := addWatchTree(watcher, root, Pool{ScanExcludeSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Fatalf("expected root and Videos watches, got %d", added)
	}
}

func TestAddWatchTreeIncludesAllDirectoriesWhenPolicyDisabled(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"Videos", "Windows"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	added, err := addWatchTree(watcher, root, Pool{ScanExcludeSystem: false})
	if err != nil {
		t.Fatal(err)
	}
	if added != 3 {
		t.Fatalf("expected root, Videos, Windows watches, got %d", added)
	}
}
