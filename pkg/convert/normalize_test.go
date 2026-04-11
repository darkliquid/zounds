package convert_test

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/convert"
)

func TestNormalizePeakTargetsRequestedDBFS(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-0.25, 0.25, -0.5, 0.5},
	}

	normalized, err := convert.Normalize(buffer, convert.NormalizeOptions{
		Mode:       convert.NormalizePeak,
		TargetDBFS: -6,
	})
	if err != nil {
		t.Fatalf("normalize peak: %v", err)
	}

	wantPeak := math.Pow(10, -6.0/20.0)
	var gotPeak float64
	for _, sample := range normalized.Data {
		if abs := math.Abs(sample); abs > gotPeak {
			gotPeak = abs
		}
	}
	if math.Abs(gotPeak-wantPeak) > 0.001 {
		t.Fatalf("expected peak %.6f, got %.6f", wantPeak, gotPeak)
	}
}

func TestNormalizeRMSTargetsRequestedDBFS(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 48000,
		Channels:   1,
		BitDepth:   24,
		Data:       []float64{-0.1, 0.1, -0.1, 0.1},
	}

	normalized, err := convert.Normalize(buffer, convert.NormalizeOptions{
		Mode:       convert.NormalizeRMS,
		TargetDBFS: -10,
	})
	if err != nil {
		t.Fatalf("normalize rms: %v", err)
	}

	var sumSq float64
	for _, sample := range normalized.Data {
		sumSq += sample * sample
	}
	gotRMS := math.Sqrt(sumSq / float64(len(normalized.Data)))
	wantRMS := math.Pow(10, -10.0/20.0)
	if math.Abs(gotRMS-wantRMS) > 0.001 {
		t.Fatalf("expected rms %.6f, got %.6f", wantRMS, gotRMS)
	}
}

func TestNormalizeRejectsClippingByDefault(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-0.8, 0.8},
	}

	if _, err := convert.Normalize(buffer, convert.NormalizeOptions{
		Mode:       convert.NormalizePeak,
		TargetDBFS: 1,
	}); err == nil {
		t.Fatal("expected clipping error")
	}
}
