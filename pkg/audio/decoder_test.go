package audio_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
)

func TestDecodeFileClassifiesUnsupportedAndCorruptFilesButNotCanceledContext(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	dir := t.TempDir()
	m4aPath := filepath.Join(dir, "clip.m4a")
	if err := os.WriteFile(m4aPath, []byte("fake-m4a"), 0o644); err != nil {
		t.Fatalf("write m4a: %v", err)
	}

	_, err = zaudio.DecodeFile(context.Background(), registry, m4aPath)
	if !errors.Is(err, zaudio.ErrUnsupportedFormat) || !zaudio.IsDecodeError(err) {
		t.Fatalf("expected IsDecodeError + ErrUnsupportedFormat for .m4a, got: %v", err)
	}

	corruptWav := filepath.Join(dir, "corrupt.wav")
	if err := os.WriteFile(corruptWav, []byte("not-a-riff-wave"), 0o644); err != nil {
		t.Fatalf("write corrupt wav: %v", err)
	}
	_, err = zaudio.DecodeFile(context.Background(), registry, corruptWav)
	if !zaudio.IsDecodeError(err) {
		t.Fatalf("expected IsDecodeError for corrupt .wav, got: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = zaudio.DecodeFile(canceledCtx, registry, corruptWav)
	if zaudio.IsDecodeError(err) {
		t.Fatalf("expected canceled context error NOT to be classified as DecodeError, got: %v", err)
	}
}
