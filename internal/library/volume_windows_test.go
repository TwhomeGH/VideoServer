//go:build windows

package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPoolVolumeIdentitySurvivesStoredPathChange(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	if pool.VolumeID == "" {
		t.Fatal("volume GUID not detected")
	}
	if pool.Filesystem == "" {
		t.Fatal("filesystem type not detected")
	}
	path := filepath.Join(pool.Path, "before.mp4")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "before.mp4", "before")
	if err != nil {
		t.Fatal(err)
	}

	fake := filepath.Join(t.TempDir(), "missing-pool")
	if _, err := store.db.Exec("UPDATE pools SET path=? WHERE id=?", fake, pool.ID); err != nil {
		t.Fatal(err)
	}
	pools, err := store.Pools()
	if err != nil || len(pools) != 1 || !pools[0].Online || pools[0].Path != fake {
		t.Fatalf("volume GUID did not recover path: %+v %v", pools, err)
	}
	file, err := store.openVideo(video)
	if err != nil {
		t.Fatalf("video unavailable after stored path change: %v", err)
	}
	file.Close()

	if err := os.WriteFile(filepath.Join(pool.Path, "after.mp4"), []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(context.Background(), pool.ID, "after.mp4", "after"); err != nil {
		t.Fatalf("import through volume GUID failed: %v", err)
	}

	scanner.stable = 0
	if err := os.WriteFile(filepath.Join(pool.Path, "scanned.mp4"), []byte("scanned"), 0600); err != nil {
		t.Fatal(err)
	}
	result := scanner.scan(context.Background(), time.Now())
	if result.Imported != 1 {
		t.Fatalf("scanner did not use volume GUID: %+v", result)
	}
}
