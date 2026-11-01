package audio_test

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestResampleAntiAliasingSuppressesUltrasonics(t *testing.T) {
	t.Parallel()

	inRate := 44100
	outRate := 8000
	duration := 1.0
	frames := int(duration * float64(inRate))

	// Signal: 1000 Hz fundamental (amplitude 0.5) + 15000 Hz high-frequency tone (amplitude 0.5)
	// Downsampling to 8000 Hz (Nyquist 4000 Hz):
	// The 15 kHz tone would alias to 1000 Hz if un-filtered!
	data := make([]float64, frames)
	for i := range data {
		tSec := float64(i) / float64(inRate)
		data[i] = 0.5*math.Sin(2*math.Pi*1000*tSec) + 0.5*math.Sin(2*math.Pi*15000*tSec)
	}

	buf := zaudio.PCMBuffer{
		SampleRate: inRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}

	resampled, err := zaudio.Resample(buf, outRate)
	if err != nil {
		t.Fatalf("Resample: %v", err)
	}

	if resampled.SampleRate != outRate {
		t.Fatalf("expected sample rate %d, got %d", outRate, resampled.SampleRate)
	}

	// Because 15 kHz was filtered out, only the 1000 Hz tone remains.
	// The RMS of a 0.5-amplitude sine wave is 0.5 / sqrt(2) = 0.3535.
	// If 15 kHz aliased and added in-phase/quadrature, RMS would be sqrt(0.3535^2 + 0.3535^2) = 0.500.
	var sumSq float64
	for _, s := range resampled.Data {
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(len(resampled.Data)))
	wantRMS := 0.5 / math.Sqrt(2)

	if math.Abs(rms-wantRMS) > 0.03 {
		t.Errorf("expected filtered RMS ~%.4f (15kHz removed), got %.4f (possible aliasing)", wantRMS, rms)
	}
}

func TestResampleUpsamplingPreservesTone(t *testing.T) {
	t.Parallel()

	inRate := 44100
	outRate := 96000
	duration := 0.5
	frames := int(duration * float64(inRate))

	data := make([]float64, frames*2) // Stereo
	for i := 0; i < frames; i++ {
		tSec := float64(i) / float64(inRate)
		val := math.Sin(2 * math.Pi * 440 * tSec)
		data[i*2] = val
		data[i*2+1] = -val
	}

	buf := zaudio.PCMBuffer{
		SampleRate: inRate,
		Channels:   2,
		BitDepth:   24,
		Data:       data,
	}

	resampled, err := zaudio.Resample(buf, outRate)
	if err != nil {
		t.Fatalf("Resample: %v", err)
	}

	if resampled.SampleRate != outRate {
		t.Fatalf("expected rate %d, got %d", outRate, resampled.SampleRate)
	}
	if resampled.Channels != 2 {
		t.Fatalf("expected 2 channels, got %d", resampled.Channels)
	}

	expectedFrames := int(math.Round(float64(frames) * float64(outRate) / float64(inRate)))
	if math.Abs(float64(resampled.Frames()-expectedFrames)) > 1 {
		t.Errorf("expected ~%d frames, got %d", expectedFrames, resampled.Frames())
	}
}

func TestResampleSameRateReturnsClone(t *testing.T) {
	t.Parallel()

	buf := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{0.1, 0.2, 0.3},
	}

	resampled, err := zaudio.Resample(buf, 44100)
	if err != nil {
		t.Fatalf("Resample: %v", err)
	}
	if len(resampled.Data) != len(buf.Data) {
		t.Fatalf("expected same data length")
	}
}
