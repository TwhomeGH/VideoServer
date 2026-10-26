package library

import (
	"context"
	"fmt"
	"testing"
)

func TestWorkStatusUsesFullQueueAndPrioritizesRunning(t *testing.T) {
	store, _, pool := scannerFixture(t)
	queue := testQueue(t, store)
	for i := range 125 {
		status := "queued"
		if i == 124 {
			status = "running"
		}
		_, err := store.db.Exec(`INSERT INTO import_jobs VALUES(?,?,?,?,?,'{}','manual',?,'queued',0,1,'','',0,?,?)`, fmt.Sprint(i), pool.ID, fmt.Sprint(i), fmt.Sprint(i), "job", status, timestamp(), timestamp())
		if err != nil {
			t.Fatal(err)
		}
	}
	result, err := queue.WorkStatus(context.Background())
	if err != nil || result.Counts["queued"] != 124 || result.Counts["running"] != 1 || len(result.Active) != 10 || result.Active[0].ID != "124" {
		t.Fatalf("bad full queue status: %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := queue.WorkStatus(ctx); err == nil {
		t.Fatal("canceled query succeeded")
	}
}
