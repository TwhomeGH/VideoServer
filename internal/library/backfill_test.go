package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func importWithoutMedia(t *testing.T, store *Store, pool Pool, name string) Video {
	t.Helper()
	if err := os.WriteFile(filepath.Join(pool.Path, name), []byte(name), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.Import(context.Background(), pool.ID, name, name)
	if err != nil {
		t.Fatal(err)
	}
	return video
}

func TestBackfillStatusShowsNextCandidate(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	video := importWithoutMedia(t, store, pool, "next-target.mp4")
	status, err := queue.BackfillStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CandidateCount != 1 || status.NextVideoID != video.ID || status.NextVideoTitle != "next-target.mp4" {
		t.Fatalf("unexpected backfill target: %+v", status)
	}
	if !status.QueueIdle {
		t.Fatalf("empty queue should be idle: %+v", status)
	}
}

func TestBackfillStatusShowsCandidatesWhileQueueBusy(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	if err := os.WriteFile(filepath.Join(pool.Path, "busy.mp4"), []byte("busy"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := queue.Enqueue(context.Background(), pool.ID, "busy.mp4", "", "manual", Metadata{}); err != nil {
		t.Fatal(err)
	}
	missing := importWithoutMedia(t, store, pool, "missing.mp4")
	status, err := queue.BackfillStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.QueueIdle || status.CandidateCount != 1 || status.NextVideoID != missing.ID {
		t.Fatalf("busy queue status: %+v", status)
	}
}

func TestBackfillDailyLimits(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	video := importWithoutMedia(t, store, pool, "daily-limit.mp4")
	queue.SetBackfillLimits(1, 1)
	if err := queue.recordBackfillUsage(context.Background(), video.Size); err != nil {
		t.Fatal(err)
	}
	status, err := queue.BackfillStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.DailyVideos != 1 || !status.LimitReached || status.DailyResetAt == "" {
		t.Fatalf("daily limit not reflected: %+v", status)
	}
	status, err = queue.ResetBackfillUsage(context.Background())
	if err != nil || status.DailyVideos != 0 || status.LimitReached {
		t.Fatalf("daily usage was not reset: %+v %v", status, err)
	}
}

func TestBackfillMissingMediaOneAtATime(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	first := importWithoutMedia(t, store, pool, "first.mp4")
	second := importWithoutMedia(t, store, pool, "second.mp4")

	job, added, err := queue.BackfillMissingMedia(context.Background())
	if err != nil || !added {
		t.Fatalf("first backfill = %v, %v", added, err)
	}
	if job.Source != "reprobe" || job.Status != "queued" || (job.VideoID != first.ID && job.VideoID != second.ID) {
		t.Fatalf("unexpected backfill job: %+v", job)
	}
	if _, added, err := queue.BackfillMissingMedia(context.Background()); err != nil || added {
		t.Fatalf("backfill should wait for the queued reprobe: %v, %v", added, err)
	}

	stop := runQueue(t, queue)
	waitJob(t, queue, job.ID, "completed")
	stop()

	otherID := first.ID
	if job.VideoID == first.ID {
		otherID = second.ID
	}
	processed, err := store.video(job.VideoID)
	if err != nil || processed.Media == nil {
		t.Fatalf("processed video media missing: %+v %v", processed, err)
	}
	remaining, err := store.video(otherID)
	if err != nil || remaining.Media != nil {
		t.Fatalf("other video should still be missing media: %+v %v", remaining, err)
	}

	next, added, err := queue.BackfillMissingMedia(context.Background())
	if err != nil || !added || next.VideoID != otherID {
		t.Fatalf("second video not scheduled: %+v, %v, %v", next, added, err)
	}
	stop = runQueue(t, queue)
	waitJob(t, queue, next.ID, "completed")
	stop()

	after, err := store.video(otherID)
	if err != nil || after.Media == nil {
		t.Fatalf("second video media missing: %+v %v", after, err)
	}
	if _, added, err := queue.BackfillMissingMedia(context.Background()); err != nil || added {
		t.Fatalf("backfill should be complete: %v, %v", added, err)
	}
}

func TestBackfillMissingMediaSkipsUnavailableAndRecorded(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)

	offline := importWithoutMedia(t, store, pool, "offline.mp4")
	if err := os.Remove(filepath.Join(pool.Path, "offline.mp4")); err != nil {
		t.Fatal(err)
	}
	changed := importWithoutMedia(t, store, pool, "changed.mp4")
	if err := os.WriteFile(filepath.Join(pool.Path, "changed.mp4"), []byte("changed and longer"), 0600); err != nil {
		t.Fatal(err)
	}
	withMedia := importWithoutMedia(t, store, pool, "media.mp4")
	if _, err := store.db.Exec("INSERT INTO video_media(video_id, details_json) VALUES(?, '{}')", withMedia.ID); err != nil {
		t.Fatal(err)
	}
	failed := importWithoutMedia(t, store, pool, "failed.mp4")
	job, err := queue.EnqueueProbe(context.Background(), failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE import_jobs SET status='failed' WHERE id=?", job.ID); err != nil {
		t.Fatal(err)
	}

	if _, added, err := queue.BackfillMissingMedia(context.Background()); err != nil || added {
		t.Fatalf("unexpected backfill: %v, %v", added, err)
	}
	if _, added, err := queue.BackfillMissingMedia(context.Background()); err != nil || added {
		t.Fatalf("skipped videos should stay skipped: %v, %v", added, err)
	}
	var count int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM import_jobs WHERE source='reprobe'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected only the failed reprobe record, got %d", count)
	}
	for _, id := range []string{offline.ID, changed.ID, failed.ID} {
		video, err := store.video(id)
		if err != nil || video.Media != nil {
			t.Fatalf("skipped video changed: %+v %v", video, err)
		}
	}
}
