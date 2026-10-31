package media

import (
	"bytes"
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

func TestTransportStreamSniff(t *testing.T) {
	tsData := make([]byte, 2048)
	for _, offset := range []int{0, 188, 376} {
		tsData[offset] = 0x47
	}
	if !LooksLikeTransportStream(bytes.NewReader(tsData), false) {
		t.Fatal("valid MPEG-TS was rejected")
	}
	if LooksLikeTransportStream(bytes.NewReader([]byte("import type { X } from './x';\n")), false) {
		t.Fatal("TypeScript source was accepted as MPEG-TS")
	}

	m2tsData := make([]byte, 2048)
	for _, offset := range []int{4, 196, 388} {
		m2tsData[offset] = 0x47
	}
	if !LooksLikeTransportStream(bytes.NewReader(m2tsData), true) {
		t.Fatal("valid M2TS was rejected")
	}
}

func TestSupportsOnlyChecksExtension(t *testing.T) {
	formats := &VideoFormats{extensions: map[string]struct{}{".ts": {}}}
	if !formats.Supports("segment.ts") {
		t.Fatal("enabled .ts extension was rejected")
	}
	if formats.Supports("segment.mp4") {
		t.Fatal("disabled .mp4 extension was accepted")
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
