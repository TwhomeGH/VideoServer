package library

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPoolDisableBlocksAccessScanAndBackfill(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "video.mp4")
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "video.mp4", "video")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		if pool.Filesystem == "" {
			t.Fatal("filesystem type not detected")
		}
	}

	disabled, err := store.SetPoolEnabled(pool.ID, false)
	if err != nil || disabled.Enabled {
		t.Fatalf("disable failed: %+v %v", disabled, err)
	}
	if _, err := store.poolPath(pool.ID); !errors.Is(err, ErrPoolDisabled) {
		t.Fatalf("disabled pool path: %v", err)
	}
	if _, err := store.openVideo(video); !errors.Is(err, ErrPoolDisabled) {
		t.Fatalf("disabled pool video opened: %v", err)
	}
	page, err := store.QueryVideos(context.Background(), VideoQuery{Page: 1, PageSize: 24})
	if err != nil || len(page.Videos) != 1 || page.Videos[0].Available || page.Videos[0].UnavailableReason != "pool_disabled" {
		t.Fatalf("disabled video status: %+v %v", page, err)
	}

	if err := os.WriteFile(filepath.Join(pool.Path, "new.mp4"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	result := scanner.scan(context.Background(), time.Now())
	if result.Imported != 0 || result.Enqueued != 0 {
		t.Fatalf("disabled pool was scanned: %+v", result)
	}
	queue := testQueue(t, store)
	if _, added, err := queue.BackfillMissingMedia(context.Background()); err != nil || added {
		t.Fatalf("disabled pool was backfilled: %v %v", added, err)
	}

	enabled, err := store.SetPoolEnabled(pool.ID, true)
	if err != nil || !enabled.Enabled {
		t.Fatalf("enable failed: %+v %v", enabled, err)
	}
	file, err := store.openVideo(video)
	if err != nil {
		t.Fatalf("enabled pool video unavailable: %v", err)
	}
	file.Close()
}

func TestPoolEnabledAPI(t *testing.T) {
	store, _, pool := scannerFixture(t)
	mux := http.NewServeMux()
	store.Register(mux)
	request := httptest.NewRequest("PATCH", "http://node/api/v1/pools/"+pool.ID, strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://node")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("disable API: %s", response.Body.String())
	}
	var updated Pool
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil || updated.ID != pool.ID || updated.Enabled {
		t.Fatalf("unexpected pool response: %+v %v", updated, err)
	}
}

func TestRefreshPoolMetadata(t *testing.T) {
	store, _, pool := scannerFixture(t)
	if _, err := store.db.Exec("UPDATE pools SET volume_id='',volume_relative='',filesystem='' WHERE id=?", pool.ID); err != nil {
		t.Fatal(err)
	}
	pools, err := store.RefreshPoolMetadata()
	if err != nil || len(pools) != 1 || pools[0].Filesystem == "" {
		t.Fatalf("metadata was not refreshed: %+v %v", pools, err)
	}
	if runtime.GOOS == "windows" && pools[0].VolumeID == "" {
		t.Fatalf("Windows volume GUID was not refreshed: %+v", pools[0])
	}
	if pools[0].VolumeStatus == "" {
		t.Fatalf("volume status missing: %+v", pools[0])
	}
}
