package library

import "testing"

func TestShouldExcludeSystemPath(t *testing.T) {
	for _, path := range []string{
		"Windows/Media/sample.mp4", "Program Files/app/video.mp4",
		"Users/alice/AppData/Local/cache/video.mp4", "$Recycle.Bin/x.mp4",
		"node_modules/pkg/sample.ts", ".git/objects/video.mp4",
	} {
		if !shouldExcludeSystemPath(path) {
			t.Fatalf("expected %q to be excluded", path)
		}
	}
	for _, path := range []string{"Videos/travel.mp4", "Movies/a/b.mkv", ".", ""} {
		if shouldExcludeSystemPath(path) {
			t.Fatalf("expected %q to remain visible", path)
		}
	}
}

func TestPoolScanExcludeSystemToggle(t *testing.T) {
	store, _, pool := scannerFixture(t)
	if !pool.ScanExcludeSystem {
		t.Fatal("new pools should exclude system folders by default")
	}
	updated, err := store.SetPoolScanExcludeSystem(pool.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ScanExcludeSystem {
		t.Fatal("scanExcludeSystem was not disabled")
	}
}
