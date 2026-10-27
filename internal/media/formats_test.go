package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDemuxers(t *testing.T) {
	available := parseDemuxers(" D  mov,mp4,m4a,3gp,3g2,mj2 QuickTime / MOV\n DE matroska,webm Matroska / WebM\n")
	for _, name := range []string{"mov", "mp4", "matroska", "webm"} {
		if !available[name] {
			t.Fatalf("missing demuxer %s in %+v", name, available)
		}
	}
}

func TestVideoFormatTransportStreamSniff(t *testing.T) {
	dir := t.TempDir()
	tsPath := filepath.Join(dir, "segment.ts")
	tsData := make([]byte, 2048)
	for _, offset := range []int{0, 188, 376} {
		tsData[offset] = 0x47
	}
	if err := os.WriteFile(tsPath, tsData, 0600); err != nil {
		t.Fatal(err)
	}
	formats := &VideoFormats{extensions: map[string]struct{}{".ts": {}}}
	if !formats.Supports(tsPath) {
		t.Fatal("valid MPEG-TS was rejected")
	}
	if err := os.WriteFile(tsPath, []byte("import type { X } from './x';\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if formats.Supports(tsPath) {
		t.Fatal("TypeScript source was accepted as MPEG-TS")
	}

	m2tsPath := filepath.Join(dir, "clip.mts")
	m2tsData := make([]byte, 2048)
	for _, offset := range []int{4, 196, 388} {
		m2tsData[offset] = 0x47
	}
	if err := os.WriteFile(m2tsPath, m2tsData, 0600); err != nil {
		t.Fatal(err)
	}
	formats.extensions[".mts"] = struct{}{}
	if !formats.Supports(m2tsPath) {
		t.Fatal("valid M2TS was rejected")
	}
}

func TestDefaultVideoFormatsExcludesAmbiguousTypeScript(t *testing.T) {
	formats := DefaultVideoFormats()
	if formats.Supports("main.ts") || formats.Supports("main.mts") {
		t.Fatal("ambiguous TypeScript extensions must not be enabled by default")
	}
	if !formats.Supports("movie.mp4") {
		t.Fatal("mp4 should be enabled by default")
	}
}
