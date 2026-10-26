package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TwhomeGH/VideoServer/internal/media"
)

func testQueue(t *testing.T, store *Store) *ImportQueue {
	t.Helper()
	queue, err := NewImportQueue(store, "ffprobe", time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	queue.probe = func(context.Context, string) (media.Info, error) {
		return media.Info{Width: 320, Height: 180, VideoCodec: "h264", DurationSeconds: 2, ProbedAt: time.Now().UTC()}, nil
	}
	return queue
}

func runQueue(t *testing.T, queue *ImportQueue) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { queue.Run(ctx); close(done) }()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("queue did not stop")
		}
	}
}

func waitJob(t *testing.T, queue *ImportQueue, id, status string) ImportJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := queue.Job(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == status {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	job, _ := queue.Job(id)
	t.Fatalf("expected %s, got %+v", status, job)
	return job
}

func TestQueueSurvivesRestartAndCommitsVideoAtomically(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), []byte("video bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	pool, err := store.AddPool("pool", root)
	if err != nil {
		t.Fatal(err)
	}
	queue := testQueue(t, store)
	job, added, err := queue.Enqueue(context.Background(), pool.ID, "video.mp4", "title", "manual", Metadata{Tags: []string{"Tag", "tag"}})
	if err != nil || !added {
		t.Fatalf("enqueue: %v", err)
	}
	duplicate, added, err := queue.Enqueue(context.Background(), pool.ID, "video.mp4", "", "scan", Metadata{})
	if err != nil || added || duplicate.ID != job.ID {
		t.Fatal("scanner created duplicate job")
	}
	if _, _, err := queue.Enqueue(context.Background(), pool.ID, "video.mp4", "replacement", "manual", Metadata{}); !errors.Is(err, ErrImportExists) {
		t.Fatal("manual metadata silently replaced existing job")
	}
	if _, err := store.db.Exec("UPDATE import_jobs SET status='running',phase='hashing' WHERE id=?", job.ID); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue = testQueue(t, store)
	recovered, err := queue.Job(job.ID)
	if err != nil || recovered.Status != "queued" {
		t.Fatal("interrupted job was not recovered")
	}
	stop := runQueue(t, queue)
	defer stop()
	finished := waitJob(t, queue, job.ID, "completed")
	video, err := store.video(finished.VideoID)
	if err != nil || video.Media == nil || video.Media.VideoCodec != "h264" || len(video.Tags) != 1 || video.Title != "title" || finished.BytesProcessed != video.Size {
		t.Fatalf("bad completed result: %+v %+v %v", video, finished, err)
	}
	jobs, err := queue.Jobs()
	if err != nil || len(jobs) != 1 {
		t.Fatal("persistent queue duplicated work")
	}
}

func TestQueueFailureRetry(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	path := filepath.Join(pool.Path, "broken.mp4")
	if err := os.WriteFile(path, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	var fail atomic.Bool
	fail.Store(true)
	queue.probe = func(context.Context, string) (media.Info, error) {
		if fail.Load() {
			return media.Info{}, errors.New("invalid video")
		}
		return media.Info{Width: 10, Height: 10, VideoCodec: "h264"}, nil
	}
	job, _, err := queue.Enqueue(context.Background(), pool.ID, "broken.mp4", "", "manual", Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	stop := runQueue(t, queue)
	defer stop()
	failed := waitJob(t, queue, job.ID, "failed")
	if failed.Error == "" {
		t.Fatal("probe error not retained")
	}
	videos, err := store.Videos()
	if err != nil || len(videos) != 0 {
		t.Fatal("failed probe created a video index")
	}
	fail.Store(false)
	if _, err := queue.Retry(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	finished := waitJob(t, queue, job.ID, "completed")
	if finished.Attempts != 2 {
		t.Fatal("retry attempts not preserved")
	}
	if _, err := queue.Retry(context.Background(), job.ID); err == nil {
		t.Fatal("completed work retried")
	}
}

func TestQueueRejectsSourceChangedDuringProbe(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	path := filepath.Join(pool.Path, "changed.mp4")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	queue.probe = func(context.Context, string) (media.Info, error) {
		err := os.WriteFile(path, []byte("changed source bytes"), 0600)
		return media.Info{Width: 10, Height: 10, VideoCodec: "h264"}, err
	}
	job, _, err := queue.Enqueue(context.Background(), pool.ID, "changed.mp4", "", "manual", Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	stop := runQueue(t, queue)
	defer stop()
	waitJob(t, queue, job.ID, "failed")
	videos, err := store.Videos()
	if err != nil || len(videos) != 0 {
		t.Fatal("changed source committed to video index")
	}
}

func TestQueueCancellationReturnsWorkToWaiting(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	if err := os.WriteFile(filepath.Join(pool.Path, "video.mp4"), []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	queue.probe = func(ctx context.Context, _ string) (media.Info, error) {
		close(started)
		<-ctx.Done()
		return media.Info{}, ctx.Err()
	}
	job, _, err := queue.Enqueue(context.Background(), pool.ID, "video.mp4", "", "manual", Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	stop := runQueue(t, queue)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		stop()
		t.Fatal("probe did not start")
	}
	stop()
	recovered, err := queue.Job(job.ID)
	if err != nil || recovered.Status != "queued" {
		t.Fatal("cancellation lost work")
	}
}

func TestScannerQueuesWithoutHashing(t *testing.T) {
	store, scanner, pool := scannerFixture(t)
	queue := testQueue(t, store)
	if err := os.WriteFile(filepath.Join(pool.Path, "scan.mp4"), []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Minute)
	scanner.scan(context.Background(), now)
	result := scanner.scan(context.Background(), now.Add(11*time.Second))
	if result.Enqueued != 1 || result.Imported != 0 {
		t.Fatalf("scanner did not enqueue: %+v", result)
	}
	videos, err := store.Videos()
	if err != nil || len(videos) != 0 {
		t.Fatal("scanner hashed before background worker ran")
	}
	jobs, err := queue.Jobs()
	if err != nil || len(jobs) != 1 {
		t.Fatal("missing queued job")
	}
	if result := scanner.scan(context.Background(), now.Add(time.Minute)); result.Enqueued != 0 {
		t.Fatal("scan enqueued duplicate")
	}
}
