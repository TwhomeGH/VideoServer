package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCoverSaveAndDelete(t *testing.T) {
	store, _, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "cover-test.mp4")
	if err := os.WriteFile(path, []byte("not a real video"), 0600); err != nil {
		t.Fatal(err)
	}
	video, err := store.importVideo(context.Background(), pool.ID, "cover-test.mp4", "cover test", Metadata{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.saveCover(context.Background(), video.ID, "image/jpeg", []byte{0xff, 0xd8, 0xff, 0xd9})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CoverUpdatedAt == "" {
		t.Fatal("cover timestamp not saved")
	}
	file, mime, _, err := store.cover(video.ID)
	if err != nil || mime != "image/jpeg" {
		t.Fatalf("cover = %q %q, %v", file, mime, err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	if _, err := store.deleteCover(context.Background(), video.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.cover(video.ID); err == nil {
		t.Fatal("cover still exists after delete")
	}
}

func TestCategoryRenameAndDelete(t *testing.T) {
	store, _, pool := scannerFixture(t)
	path := filepath.Join(pool.Path, "category-test.mp4")
	if err := os.WriteFile(path, []byte("not a real video"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.importVideo(context.Background(), pool.ID, "category-test.mp4", "category test", Metadata{Category: "Action"}, nil); err != nil {
		t.Fatal(err)
	}
	categories, err := store.Categories(context.Background())
	if err != nil || len(categories) != 1 || categories[0].Name != "Action" || categories[0].VideoCount != 1 {
		t.Fatalf("categories = %+v, %v", categories, err)
	}
	renamed, err := store.RenameCategory(context.Background(), categories[0].ID, "Drama")
	if err != nil || renamed.Name != "Drama" {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	categories, err = store.Categories(context.Background())
	if err != nil || categories[0].Name != "Drama" || categories[0].VideoCount != 1 {
		t.Fatalf("renamed categories = %+v, %v", categories, err)
	}
	if err := store.DeleteCategory(context.Background(), categories[0].ID); err != nil {
		t.Fatal(err)
	}
	categories, err = store.Categories(context.Background())
	if err != nil || len(categories) != 0 {
		t.Fatalf("categories after delete = %+v, %v", categories, err)
	}
}
