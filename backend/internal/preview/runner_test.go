package preview

import (
	"image"
	"testing"
)

func TestResizeImageContain(t *testing.T) {
	t.Parallel()

	src := image.NewRGBA(image.Rect(0, 0, 640, 320))
	dst := resizeImageContain(src, 320, 320)
	if got := dst.Bounds().Dx(); got != 320 {
		t.Fatalf("width mismatch: got %d want 320", got)
	}
	if got := dst.Bounds().Dy(); got != 160 {
		t.Fatalf("height mismatch: got %d want 160", got)
	}

	small := image.NewRGBA(image.Rect(0, 0, 16, 16))
	if resizeImageContain(small, 320, 320) != small {
		t.Fatalf("small images should be returned unchanged")
	}
}

func TestIsVideoFile(t *testing.T) {
	t.Parallel()

	videoMime := "video/mp4"
	if !isVideoFile(fileForMetadata{MimeType: &videoMime}) {
		t.Fatalf("video mime should be detected")
	}

	videoExt := "webm"
	if !isVideoFile(fileForMetadata{Extension: &videoExt}) {
		t.Fatalf("video extension should be detected")
	}

	textExt := "txt"
	if isVideoFile(fileForMetadata{Extension: &textExt}) {
		t.Fatalf("text extension should not be video")
	}
}
