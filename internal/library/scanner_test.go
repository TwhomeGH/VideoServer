package library

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func scannerFixture(t *testing.T) (*Store, *Scanner, Pool) {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	root := filepath.Join(t.TempDir(), "media")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	pool, err := store.AddPool("pool", root)
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(store, true, 30*time.Second, 10*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return store, scanner, pool
}

func TestScanWaitsForStabilityAndPreservesMetadata(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	nested := filepath.Join(pool.Path, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "新增.MP4")
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "partial.mp4.part"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(20 * time.Second)
	first := scanner.scan(context.Background(), now)
	if first.Imported != 0 || first.Pending != 1 {
		t.Fatalf("first: %+v", first)
	}
	// 檔案在兩次觀察之間繼續寫入，即使經過十秒也必須重新計時。
	if err := os.WriteFile(path, []byte("still writing changed size"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := scanner.scan(context.Background(), now.Add(11*time.Second))
	if changed.Imported != 0 || changed.Pending != 1 {
		t.Fatalf("changed: %+v", changed)
	}
	stable := scanner.scan(context.Background(), now.Add(22*time.Second))
	if stable.Imported != 1 || stable.Failed != 0 {
		t.Fatalf("stable: %+v", stable)
	}
	videos, err := store.Videos()
	if err != nil || len(videos) != 1 {
		t.Fatalf("videos: %v %v", videos, err)
	}
	if videos[0].Title != "新增.MP4" || videos[0].WorkType != "unknown" {
		t.Fatal("automatic metadata must remain unconfirmed")
	}
	if _, err := store.UpdateMetadata(context.Background(), videos[0].ID, "custom title", Metadata{Description: "manual description", Tags: []string{"tag"}}); err != nil {
		t.Fatal(err)
	}
	repeated := scanner.scan(context.Background(), now.Add(40*time.Second))
	if repeated.Imported != 0 || repeated.Skipped != 1 {
		t.Fatalf("repeat: %+v", repeated)
	}
	saved, err := store.video(videos[0].ID)
	if err != nil || saved.Title != "custom title" || saved.Description != "manual description" {
		t.Fatal("scan overwrote manual metadata")
	}
	moved := pool.Path + "-offline"
	if err := os.Rename(pool.Path, moved); err != nil {
		t.Fatal(err)
	}
	defer os.Rename(moved, pool.Path)
	offline := scanner.scan(context.Background(), now.Add(time.Minute))
	if offline.OfflinePools != 1 {
		t.Fatalf("offline: %+v", offline)
	}
	videos, err = store.Videos()
	if err != nil || len(videos) != 1 {
		t.Fatal("offline scan removed index")
	}
}

func TestScanSkipsManualImportAndUnsafeLinks(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "manual.mp4")
	if err := os.WriteFile(path, []byte("manually indexed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(context.Background(), pool.ID, "manual.mp4", "manual"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	// Windows 缺少建立連結權限時，仍驗證手動匯入去重；路徑邊界另有獨立測試。
	_ = os.Symlink(outside, filepath.Join(pool.Path, "link.mp4"))
	result := scanner.scan(context.Background(), time.Now().Add(time.Minute))
	if result.Imported != 0 || result.Pending != 0 || result.Skipped != 1 {
		t.Fatalf("unexpected scan: %+v", result)
	}
}

func TestScanReconciliationCountsChangedAndDeleted(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	scanner.stable = 0
	path := filepath.Join(pool.Path, "video.mp4")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "video.mp4", "video")
	if err != nil {
		t.Fatal(err)
	}
	initial := scanner.scan(context.Background(), time.Now())
	if initial.Changed != 0 || initial.Deleted != 0 {
		t.Fatalf("unexpected initial diff: %+v", initial)
	}
	if err := os.WriteFile(path, []byte("modified and longer"), 0600); err != nil {
		t.Fatal(err)
	}
	changed := scanner.scan(context.Background(), time.Now())
	if changed.Changed != 1 || changed.Deleted != 0 {
		t.Fatalf("changed diff: %+v", changed)
	}
	afterChange, err := store.video(video.ID)
	if err != nil || afterChange.FileHash != video.FileHash {
		t.Fatal("changed source modified the index")
	}
	page, err := store.QueryVideos(context.Background(), VideoQuery{Page: 1, PageSize: 24})
	if err != nil || len(page.Videos) != 1 || page.Videos[0].UnavailableReason != "source_changed" {
		t.Fatalf("changed status: %+v %v", page, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deleted := scanner.scan(context.Background(), time.Now())
	if deleted.Changed != 0 || deleted.Deleted != 1 {
		t.Fatalf("deleted diff: %+v", deleted)
	}
	page, err = store.QueryVideos(context.Background(), VideoQuery{Page: 1, PageSize: 24})
	if err != nil || len(page.Videos) != 1 || page.Videos[0].UnavailableReason != "source_missing" {
		t.Fatalf("deleted status: %+v %v", page, err)
	}
	if err := os.WriteFile(filepath.Join(pool.Path, "new.mp4"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	added := scanner.scan(context.Background(), time.Now())
	if added.Imported != 1 || added.Deleted != 1 {
		t.Fatalf("new file diff: %+v", added)
	}
}

func TestScannerManualQueueAndShutdown(t *testing.T) {
	_, scanner, _ := scannerFixture(t)
	scanner.status.Automatic = false
	if !scanner.Request() || scanner.Request() {
		t.Fatal("queued requests were not coalesced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { scanner.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for scanner.Status().Last == nil {
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("manual scan did not run")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop")
	}
}

func TestScannerAutomaticallyImportsNewFile(t *testing.T) {
	_, scanner, pool := scannerFixture(t)
	scanner.interval = 20 * time.Millisecond
	scanner.stable = 20 * time.Millisecond
	if err := os.WriteFile(filepath.Join(pool.Path, "automatic.mp4"), []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { scanner.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.After(2 * time.Second)
	for {
		status := scanner.Status()
		if status.Last != nil && status.Last.Imported == 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("periodic scanner did not import the new file")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
