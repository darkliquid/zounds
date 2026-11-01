package audio_test

import (
	"math"
	"testing"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestPCMBufferDurationAndFrames(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 48000,
		Channels:   2,
		BitDepth:   16,
		Data:       make([]float64, 48000*2),
	}

	if frames := buffer.Frames(); frames != 48000 {
		t.Fatalf("expected 48000 frames, got %d", frames)
	}

	if duration := buffer.Duration(); duration != time.Second {
		t.Fatalf("expected 1s duration, got %s", duration)
	}
}

func TestPCMBufferValidate(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-0.5, 0.0, 0.5},
	}

	if err := buffer.Validate(); err != nil {
		t.Fatalf("validate buffer: %v", err)
	}
}

func TestPCMBufferValidateRejectsNaNAndInf(t *testing.T) {
	t.Parallel()

	for name, bad := range map[string]float64{
		"NaN":  math.NaN(),
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
	} {
		buf := zaudio.PCMBuffer{
			SampleRate: 44100,
			Channels:   1,
			BitDepth:   32,
			Data:       []float64{0.0, bad, 0.25},
		}
		if err := buf.Validate(); err == nil {
			t.Fatalf("expected Validate() to reject %s", name)
		}
	}
}
