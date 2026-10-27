package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

func TestQueueSerialFIFOAndContinuesWithoutIdleDelay(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	entered := make(chan string, 3)
	gate := make(chan struct{})
	queue.probe = func(ctx context.Context, path string) (media.Info, error) {
		name := filepath.Base(path)
		entered <- name
		if name == "0.mp4" {
			select {
			case <-gate:
			case <-ctx.Done():
				return media.Info{}, ctx.Err()
			}
		}
		return media.Info{Width: 320, Height: 180, VideoCodec: "h264"}, nil
	}
	jobs := []ImportJob{}
	for i := range 3 {
		name := fmt.Sprintf("%d.mp4", i)
		if err := os.WriteFile(filepath.Join(pool.Path, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		job, _, err := queue.Enqueue(context.Background(), pool.ID, name, name, "manual", Metadata{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec("UPDATE import_jobs SET created_at=? WHERE id=?", fmt.Sprintf("2026-10-27T00:00:0%d.000000000Z", i), job.ID); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	stop := runQueue(t, queue)
	defer stop()
	select {
	case name := <-entered:
		if name != "0.mp4" {
			t.Fatal("not FIFO")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queue did not start")
	}
	state, err := queue.WorkStatus(context.Background())
	if err != nil || state.Counts["running"] != 1 || state.Counts["queued"] != 2 {
		t.Fatalf("not serial: %+v %v", state, err)
	}
	select {
	case <-entered:
		t.Fatal("started another job before first completed")
	default:
	}
	close(gate)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for _, expected := range []string{"1.mp4", "2.mp4"} {
		select {
		case name := <-entered:
			if name != expected {
				t.Fatalf("expected %s, got %s", expected, name)
			}
		case <-deadline.C:
			t.Fatal("queue inserted idle delay between ready jobs")
		}
	}
	waitJob(t, queue, jobs[2].ID, "completed")
}

func TestLightweightJobListWorksWithOfflineSource(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	path := filepath.Join(pool.Path, "video.mp4")
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	job, _, err := queue.Enqueue(context.Background(), pool.ID, "video.mp4", "title", "manual", Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".offline"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	store.Register(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/import-jobs", nil))
	var result []ImportJob
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || len(result) != 1 || result[0].ID != job.ID || result[0].Status != "queued" {
		t.Fatalf("bad job snapshot: %s", response.Body.String())
	}
}
