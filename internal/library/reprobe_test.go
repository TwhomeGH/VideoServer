package library

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

func TestReprobePreservesIdentityAndConcurrentMetadata(t *testing.T) {
	store, _, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "old.mp4")
	if err := os.WriteFile(path, []byte("old video"), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, "old.mp4", "原標題", Metadata{Tags: []string{"舊標記"}})
	if err != nil {
		t.Fatal(err)
	}
	queue := testQueue(t, store)
	job, err := queue.EnqueueProbe(context.Background(), video.ID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := queue.EnqueueProbe(context.Background(), video.ID)
	if err != nil || duplicate.ID != job.ID {
		t.Fatal("duplicate probe job")
	}
	updated := Metadata{Description: "處理期間更新", Tags: []string{"新標記"}, Category: "教育", Adult: true}
	if _, err := store.UpdateMetadata(context.Background(), video.ID, "新標題", updated); err != nil {
		t.Fatal(err)
	}
	want, _ := store.video(video.ID)
	// 模擬程序中斷後重新開啟佇列，工作必須仍可完成。
	if _, err := store.db.Exec("UPDATE import_jobs SET status='running' WHERE id=?", job.ID); err != nil {
		t.Fatal(err)
	}
	queue = testQueue(t, store)
	stop := runQueue(t, queue)
	waitJob(t, queue, job.ID, "completed")
	stop()
	after, err := store.video(video.ID)
	if err != nil || after.Media == nil {
		t.Fatalf("media missing: %+v %v", after, err)
	}
	if after.ID != video.ID || after.FileHash != video.FileHash || after.PublicationID != video.PublicationID || after.ReplicaID != video.ReplicaID || after.ImportedAt != video.ImportedAt || after.Title != want.Title || !reflect.DeepEqual(after.Metadata, want.Metadata) {
		t.Fatal("reprobe changed identity or descriptions")
	}
	originalMedia := *after.Media
	queue.probe = func(context.Context, string) (media.Info, error) { return media.Info{}, errors.New("probe failed") }
	job, err = queue.EnqueueProbe(context.Background(), video.ID)
	if err != nil {
		t.Fatal(err)
	}
	stop = runQueue(t, queue)
	waitJob(t, queue, job.ID, "failed")
	stop()
	after, _ = store.video(video.ID)
	if !reflect.DeepEqual(*after.Media, originalMedia) {
		t.Fatal("failed probe replaced old media")
	}
	queue.probe = func(context.Context, string) (media.Info, error) {
		return media.Info{Width: 640, Height: 360, VideoCodec: "h264", ProbedAt: time.Now().UTC()}, nil
	}
	if _, err := queue.Retry(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	stop = runQueue(t, queue)
	waitJob(t, queue, job.ID, "completed")
	stop()
	after, _ = store.video(video.ID)
	if after.Media.Width != 640 {
		t.Fatal("retry did not replace media")
	}
	mux := http.NewServeMux()
	store.Register(mux)
	for _, test := range []struct {
		id, origin string
		status     int
	}{{"missing", "", 404}, {video.ID, "http://other.example", 403}, {video.ID, "", 202}} {
		request := httptest.NewRequest("POST", "/api/v1/videos/"+test.id+"/probe", strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
	}
}

func TestReprobeRejectsChangedAndOfflineSources(t *testing.T) {
	for _, mode := range []string{"offline", "hash", "during-probe"} {
		t.Run(mode, func(t *testing.T) {
			store, _, pool := scannerFixture(t)
			path := filepath.Join(pool.Path, "video.mp4")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			video, err := store.Import(context.Background(), pool.ID, "video.mp4", "title")
			if err != nil {
				t.Fatal(err)
			}
			stat, _ := os.Stat(path)
			queue := testQueue(t, store)
			job, err := queue.EnqueueProbe(context.Background(), video.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "offline":
				if err := os.Rename(path, path+".offline"); err != nil {
					t.Fatal(err)
				}
			case "hash":
				if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, stat.ModTime(), stat.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "during-probe":
				queue.probe = func(context.Context, string) (media.Info, error) {
					if err := os.WriteFile(path, []byte("changed while probing"), 0600); err != nil {
						return media.Info{}, err
					}
					return media.Info{Width: 100, Height: 100, VideoCodec: "h264"}, nil
				}
			}
			stop := runQueue(t, queue)
			defer stop()
			waitJob(t, queue, job.ID, "failed")
			after, err := store.video(video.ID)
			if err != nil || after.Media != nil || after.FileHash != video.FileHash {
				t.Fatalf("changed source committed: %+v %v", after, err)
			}
		})
	}
}
