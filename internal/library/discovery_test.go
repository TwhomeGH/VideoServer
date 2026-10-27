package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoveredMediaLifecycle(t *testing.T) {
	store, _, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "found.mp4")
	if err := os.WriteFile(path, []byte("found"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	changed, err := store.upsertDiscovered(context.Background(), pool.ID, "found.mp4", info.Size(), info.ModTime(), at)
	if err != nil || !changed {
		t.Fatalf("upsert discovered: changed=%v err=%v", changed, err)
	}
	changed, err = store.upsertDiscovered(context.Background(), pool.ID, "found.mp4", info.Size(), info.ModTime(), at)
	if err != nil || changed {
		t.Fatalf("unchanged discovery should not count again: changed=%v err=%v", changed, err)
	}
	found, err := store.hasDiscovered(context.Background(), pool.ID, "found.mp4")
	if err != nil || !found {
		t.Fatalf("hasDiscovered = %v, %v", found, err)
	}
	page, err := store.ListDiscovered(context.Background(), pool.ID, "", 1, 50)
	if err != nil || page.Total != 1 || page.Items[0].RelativePath != "found.mp4" {
		t.Fatalf("ListDiscovered = %+v, %v", page, err)
	}
	if err := store.markDiscoveredMissing(context.Background(), pool.ID, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	page, err = store.ListDiscovered(context.Background(), pool.ID, "", 1, 50)
	if err != nil || page.Total != 0 {
		t.Fatalf("missing discovery still visible: %+v, %v", page, err)
	}
}
