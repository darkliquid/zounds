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

func TestNormalizeLUFSTargetsRequestedLUFS(t *testing.T) {
	t.Parallel()

	sampleRate := 48000
	duration := 2.0
	frames := int(duration * float64(sampleRate))
	data := make([]float64, frames*2) // Stereo

	// 1 kHz sine wave at -10 dBFS peak (~0.3162)
	peak := math.Pow(10, -10.0/20.0)
	for i := 0; i < frames; i++ {
		val := peak * math.Sin(2*math.Pi*1000.0*float64(i)/float64(sampleRate))
		data[i*2] = val
		data[i*2+1] = val
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   2,
		BitDepth:   16,
		Data:       data,
	}

	initialLUFS := zaudio.MeasureLUFS(buffer)

	targetLUFS := -14.0
	normalized, err := convert.Normalize(buffer, convert.NormalizeOptions{
		Mode:          convert.NormalizeLUFS,
		TargetDBFS:    targetLUFS,
		AllowClipping: false,
	})
	if err != nil {
		t.Fatalf("normalize LUFS: %v", err)
	}

	finalLUFS := zaudio.MeasureLUFS(normalized)
	if math.Abs(finalLUFS-targetLUFS) > 0.1 {
		t.Errorf("expected final LUFS ~%.2f, got %.2f (initial was %.2f)", targetLUFS, finalLUFS, initialLUFS)
	}
}
