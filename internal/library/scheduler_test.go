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

func TestBackfillDisabledAndBackpressure(t *testing.T) {
	store, _, pool := scannerFixture(t)
	importWithoutMedia(t, store, pool, "missing.mp4")
	queue := testQueue(t, store)
	ctx := context.Background()
	queue.RunBackfill(ctx, time.Hour, false)
	status, err := queue.BackfillStatus(ctx)
	if err != nil || status.Enabled {
		t.Fatalf("disabled backfill status: %+v %v", status, err)
	}

	if err := os.WriteFile(filepath.Join(pool.Path, "queued.mp4"), []byte("queued"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := queue.Enqueue(ctx, pool.ID, "queued.mp4", "", "manual", Metadata{}); err != nil {
		t.Fatal(err)
	}
	if _, added, err := queue.BackfillMissingMedia(ctx); err != nil || added {
		t.Fatalf("backfill ran while another job was queued: %v %v", added, err)
	}
}

func TestBackfillEnabledExposesNextRun(t *testing.T) {
	store, _, pool := scannerFixture(t)
	importWithoutMedia(t, store, pool, "missing.mp4")
	queue := testQueue(t, store)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { queue.RunBackfill(ctx, time.Second, true); close(done) }()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		status, err := queue.BackfillStatus(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if status.NextCheckAt != "" {
			if !status.Enabled || status.LastVideoID == "" {
				t.Fatalf("backfill status incomplete: %+v", status)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("backfill did not expose next run: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestScannerTierAndPoolScan(t *testing.T) {
	_, scanner, pool := scannerFixture(t)
	scanner.interval = 20 * time.Millisecond
	scanner.SetIdleMax(time.Hour)
	ctx := context.Background()
	scanner.runDue(ctx)
	readState := func() poolScanState {
		scanner.mu.Lock()
		defer scanner.mu.Unlock()
		return scanner.states[pool.ID]
	}
	state := readState()
	if state.tier != "hot" || state.interval != 20*time.Millisecond {
		t.Fatalf("initial tier: %+v", state)
	}

	setAge := func(age time.Duration) {
		scanner.mu.Lock()
		state = scanner.states[pool.ID]
		state.lastActivity = time.Now().Add(-age)
		state.next = time.Now().Add(-time.Second)
		scanner.states[pool.ID] = state
		scanner.mu.Unlock()
		scanner.runDue(ctx)
	}
	setAge(2 * time.Hour)
	state = readState()
	if state.tier != "warm" || state.interval != 5*time.Minute {
		t.Fatalf("warm tier: %+v", state)
	}
	setAge(7 * time.Hour)
	state = readState()
	if state.tier != "warm" || state.interval != 15*time.Minute {
		t.Fatalf("slow warm tier: %+v", state)
	}
	setAge(2 * 24 * time.Hour)
	state = readState()
	if state.tier != "cold" || state.interval != 30*time.Minute {
		t.Fatalf("cold tier: %+v", state)
	}
	setAge(8 * 24 * time.Hour)
	state = readState()
	if state.tier != "dormant" || state.interval != time.Hour {
		t.Fatalf("dormant tier: %+v", state)
	}

	scanner.mu.Lock()
	state = scanner.states[pool.ID]
	state.lastActivity = time.Now().Add(-8 * 24 * time.Hour)
	state.next = time.Now().Add(-time.Second)
	scanner.states[pool.ID] = state
	scanner.mu.Unlock()
	if !scanner.RequestPool(pool.ID) {
		t.Fatal("pool scan request was not queued")
	}
	scanner.runDue(ctx)
	state = readState()
	if state.tier != "hot" || state.interval != 20*time.Millisecond {
		t.Fatalf("manual pool scan did not promote: %+v", state)
	}
}

func TestHashSourceRateLimit(t *testing.T) {
	store, _, _ := scannerFixture(t)
	path := filepath.Join(t.TempDir(), "rate.mp4")
	if err := os.WriteFile(path, make([]byte, 1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	store.hashLimitBytesPerSecond = 1024 * 1024
	started := time.Now()
	if _, err := store.hashSource(context.Background(), file, 1024*1024, ""); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < 700*time.Millisecond {
		t.Fatalf("hash was not rate limited: %v", elapsed)
	}
}

func TestScannerExposesNextRun(t *testing.T) {
	store, _, _ := scannerFixture(t)
	scanner := NewScanner(store, true, 20*time.Millisecond, 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { scanner.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := scanner.Status()
		if status.Last != nil && status.NextRunAt != "" {
			if !status.Automatic || status.NextRunInSeconds < 0 {
				t.Fatalf("scan status incomplete: %+v", status)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scanner did not expose next run: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
