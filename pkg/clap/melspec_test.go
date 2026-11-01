package clap

import (
	"math"
	"testing"
)

func TestFeatureExtractorConfig_NumFrames(t *testing.T) {
	cfg := DefaultFeatureExtractorConfig()
	got := cfg.NumFrames()
	// 1 + 480000/480 = 1001
	want := 1001
	if got != want {
		t.Errorf("NumFrames() = %d, want %d", got, want)
	}
}

func TestNewFeatureExtractor_InvalidConfig(t *testing.T) {
	bad := FeatureExtractorConfig{} // all zeros
	if _, err := NewFeatureExtractor(bad); err == nil {
		t.Error("expected error for zero config")
	}
}

func TestExtract_OutputShape(t *testing.T) {
	cfg := FeatureExtractorConfig{
		SampleRate:    16000,
		NumMelBins:    16,
		FFTWindowSize: 256,
		HopLength:     80,
		FreqMin:       50,
		FreqMax:       8000,
		MaxSamples:    16000, // 1 s at 16 kHz
	}
	fe, err := NewFeatureExtractor(cfg)
	if err != nil {
		t.Fatalf("NewFeatureExtractor: %v", err)
	}

	// 1 s of 440 Hz sine wave at 16 kHz
	samples := make([]float64, 16000)
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / 16000)
	}

	out, err := fe.Extract(samples, 16000)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	wantLen := cfg.NumMelBins * cfg.NumFrames()
	if len(out) != wantLen {
		t.Errorf("len(out) = %d, want %d", len(out), wantLen)
	}

	// Values should be raw dB (finite, not NaN or Inf).
	for i, v := range out {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Errorf("out[%d] = %v is NaN/Inf", i, v)
			break
		}
	}
}

func TestMelFilterBankSlaney_Shape(t *testing.T) {
	filters := melFilterBankSlaney(64, 1024, 48000, 50, 14000)
	if len(filters) != 64 {
		t.Errorf("got %d filters, want 64", len(filters))
	}
	binCount := 1024/2 + 1
	for i, f := range filters {
		if len(f) != binCount {
			t.Errorf("filter[%d] has len %d, want %d", i, len(f), binCount)
		}
		var maxW float64
		for _, w := range f {
			if w < 0 {
				t.Errorf("filter[%d] has negative weight %f", i, w)
			}
			if w > maxW {
				maxW = w
			}
		}
		if maxW <= 0 {
			t.Errorf("filter[%d] is all zeros", i)
		}
	}
}

func TestFeatureExtractor_RepeatPad(t *testing.T) {
	cfg := FeatureExtractorConfig{
		SampleRate:    16000,
		NumMelBins:    16,
		FFTWindowSize: 256,
		HopLength:     80,
		FreqMin:       50,
		FreqMax:       8000,
		MaxSamples:    16000, // 1 s
	}
	fe, err := NewFeatureExtractor(cfg)
	if err != nil {
		t.Fatalf("NewFeatureExtractor: %v", err)
	}

	// 100 samples short audio
	shortAudio := make([]float64, 100)
	for i := range shortAudio {
		shortAudio[i] = float64(i) / 100.0
	}

	out, err := fe.Extract(shortAudio, 16000)
	if err != nil {
		t.Fatalf("Extract shortAudio: %v", err)
	}
	if len(out) != cfg.NumMelBins*cfg.NumFrames() {
		t.Errorf("output len = %d, want %d", len(out), cfg.NumMelBins*cfg.NumFrames())
	}
}

func TestFeatureExtractor_ConcurrentSafety(t *testing.T) {
	cfg := FeatureExtractorConfig{
		SampleRate:    16000,
		NumMelBins:    16,
		FFTWindowSize: 256,
		HopLength:     80,
		FreqMin:       50,
		FreqMax:       8000,
		MaxSamples:    1600,
	}
	fe, err := NewFeatureExtractor(cfg)
	if err != nil {
		t.Fatalf("NewFeatureExtractor: %v", err)
	}

	const goroutines = 10
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(seed float64) {
			samples := make([]float64, 1600)
			for i := range samples {
				samples[i] = math.Sin(2 * math.Pi * (200 + seed*50) * float64(i) / 16000)
			}
			_, err := fe.Extract(samples, 16000)
			errCh <- err
		}(float64(g))
	}

	for g := 0; g < goroutines; g++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent extract failed: %v", err)
		}
	}
}

func TestResampleLinear(t *testing.T) {
	// Down-sample from 44100 to 22050 (half).
	src := make([]float64, 441)
	for i := range src {
		src[i] = float64(i)
	}
	dst := resampleLinear(src, 44100, 22050)
	if len(dst) != 221 {
		t.Errorf("resample to half length: got %d, want 221", len(dst))
	}

	// Same rate → identity copy.
	src2 := []float64{1, 2, 3}
	dst2 := resampleLinear(src2, 16000, 16000)
	for i, v := range src2 {
		if dst2[i] != v {
			t.Errorf("same-rate resample changed value at %d", i)
		}
	}
}

func TestMelFilterBankHTK_Shape(t *testing.T) {
	filters := melFilterBankHTK(64, 1024, 48000, 50, 14000)
	if len(filters) != 64 {
		t.Errorf("got %d filters, want 64", len(filters))
	}
	binCount := 1024/2 + 1
	for i, f := range filters {
		if len(f) != binCount {
			t.Errorf("filter[%d] has len %d, want %d", i, len(f), binCount)
		}
	}
}

func TestReorderMelFrameToFrameMel(t *testing.T) {
	src := []float32{
		0, 1, 2,
		10, 11, 12,
	}
	dst := make([]float32, len(src))

	reorderMelFrameToFrameMel(dst, src, 2, 3)

	want := []float32{
		0, 10,
		1, 11,
		2, 12,
	}
	for i, got := range dst {
		if got != want[i] {
			t.Fatalf("dst[%d] = %v, want %v", i, got, want[i])
		}
	}
}
