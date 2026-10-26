package buildinfo

import "testing"

func TestCurrentReadsImageMetadata(t *testing.T) {
	t.Setenv("DVN_IMAGE_BUILT_AT", "2026-10-27T03:28:05Z")
	t.Setenv("DVN_IMAGE_SOURCE_DIGEST", "0123456789abcdef")
	identity := Current("video-node")
	if identity.ImageBuildTime != "2026-10-27T03:28:05Z" {
		t.Fatalf("unexpected image build time: %q", identity.ImageBuildTime)
	}
	if identity.ImageSourceDigest != "0123456789abcdef" {
		t.Fatalf("unexpected image source digest: %q", identity.ImageSourceDigest)
	}
}

func TestCurrentMarksMissingImageMetadataUnknown(t *testing.T) {
	t.Setenv("DVN_IMAGE_BUILT_AT", "")
	t.Setenv("DVN_IMAGE_SOURCE_DIGEST", "")
	identity := Current("video-node")
	if identity.ImageBuildTime != "unknown" || identity.ImageSourceDigest != "unknown" {
		t.Fatalf("missing image metadata not marked unknown: %+v", identity)
	}
}
