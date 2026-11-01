package clap

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestIsInvalidAttentionMaskInputError(t *testing.T) {
	t.Parallel()

	if !isInvalidAttentionMaskInputError(errors.New("Error running network: Invalid input name: attention_mask")) {
		t.Fatal("expected attention_mask error to be detected")
	}
	if isInvalidAttentionMaskInputError(errors.New("some other error")) {
		t.Fatal("did not expect unrelated error to be detected")
	}
}

func TestMakeCStringArrayNullTerminatesNames(t *testing.T) {
	t.Parallel()

	bufs, ptrs := makeCStringArray([]string{"input_ids", "attention_mask"})
	if len(bufs) != 2 || len(ptrs) != 2 {
		t.Fatalf("unexpected lengths: bufs=%d ptrs=%d", len(bufs), len(ptrs))
	}
	if string(bufs[0]) != "input_ids\x00" || bufs[0][len(bufs[0])-1] != 0 {
		t.Fatalf("unexpected first C string: %q", bufs[0])
	}
	if ptrs[0] != &bufs[0][0] || ptrs[1] != &bufs[1][0] {
		t.Fatal("expected pointer slice to address backing byte slices")
	}
}

func TestEnsureORTDoesNotLatchFailureOnMissingLibrary(t *testing.T) {
	err := ensureORT(filepath.Join(t.TempDir(), "does-not-exist-onnxruntime.so"))
	if err == nil {
		t.Fatal("expected error for missing ONNX Runtime shared library")
	}

	// Verify state did not latch as loaded after failed open.
	ortMu.Lock()
	loaded := ortState != nil
	ortMu.Unlock()
	if loaded {
		t.Fatal("expected ortState to remain nil after failed initialization")
	}
}

func TestLocalONNXRuntimeEndToEndWhenAssetsPresent(t *testing.T) {
	modelDir := filepath.Clean("../../onnx")
	for _, required := range []string{
		filepath.Join(modelDir, "audio_model.onnx"),
		filepath.Join(modelDir, "text_model.onnx"),
		filepath.Join(modelDir, "tokenizer.json"),
	} {
		if _, err := os.Stat(required); err != nil {
			t.Skipf("skipping live ONNX Runtime smoke test (%s not present)", required)
		}
	}

	cfg := DefaultConfig()
	if err := ensureORT(""); err != nil {
		fallbackLib := filepath.Clean("../../onnxruntime.so")
		if fallbackErr := ensureORT(fallbackLib); fallbackErr != nil {
			t.Skipf("skipping live ONNX Runtime smoke test (no loadable ORT shared library: default=%v, fallback=%v)", err, fallbackErr)
		}
		cfg.LibPath = fallbackLib
	}

	model, err := NewModel(modelDir, cfg)
	if err != nil {
		t.Fatalf("NewModel with purego ORT failed: %v", err)
	}
	defer func() {
		if closeErr := model.Close(); closeErr != nil {
			t.Fatalf("Close failed: %v", closeErr)
		}
	}()

	const sampleRate = 48000
	data := make([]float64, sampleRate/2)
	for i := range data {
		data[i] = 0.5 * math.Sin(2*math.Pi*60*float64(i)/sampleRate)
	}
	scores, err := model.ClassifyAudio(context.Background(), zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}, []string{"bass drum kick", "hi hat cymbal"})
	if err != nil {
		t.Fatalf("ClassifyAudio failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 label scores, got %d", len(scores))
	}
	if scores[0].Label != "bass drum kick" {
		t.Fatalf("expected top label 'bass drum kick', got %q", scores[0].Label)
	}
	for _, s := range scores {
		if math.IsNaN(float64(s.Score)) || math.IsInf(float64(s.Score), 0) {
			t.Fatalf("invalid score for %q: %v", s.Label, s.Score)
		}
	}
}
