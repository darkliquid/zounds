package convert_test

import (
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/convert"
)

func TestResampleLinearUpsamplesInterleavedAudio(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 2,
		Channels:   2,
		BitDepth:   16,
		Data:       []float64{0, 0.25, 1, 0.75},
	}

	resampled, err := convert.ResampleLinear(buffer, convert.ResampleOptions{TargetSampleRate: 4})
	if err != nil {
		t.Fatalf("resample linear: %v", err)
	}

	if resampled.SampleRate != 4 {
		t.Fatalf("expected sample rate 4, got %d", resampled.SampleRate)
	}

	expected := []float64{0, 0.25, 0.5, 0.5, 1, 0.75, 1, 0.75}
	for i, want := range expected {
		if resampled.Data[i] != want {
			t.Fatalf("sample %d: expected %f, got %f", i, want, resampled.Data[i])
		}
	}
}

func TestResampleLinearDownsamplesAudio(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 4,
		Channels:   1,
		BitDepth:   24,
		Data:       []float64{0, 0.33, 0.66, 1},
	}

	resampled, err := convert.ResampleLinear(buffer, convert.ResampleOptions{TargetSampleRate: 2})
	if err != nil {
		t.Fatalf("resample linear: %v", err)
	}

	expected := []float64{0, 0.66}
	for i, want := range expected {
		if resampled.Data[i] != want {
			t.Fatalf("sample %d: expected %f, got %f", i, want, resampled.Data[i])
		}
	}
}

func TestResampleLinearRejectsInvalidTargetRate(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 4,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{0, 1},
	}

	if _, err := convert.ResampleLinear(buffer, convert.ResampleOptions{}); err == nil {
		t.Fatal("expected invalid target sample rate error")
	}
}
