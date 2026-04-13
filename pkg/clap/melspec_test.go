package clap

import (
	"math"
	"testing"
)

func TestFeatureExtractorConfig_NumFrames(t *testing.T) {
	cfg := DefaultFeatureExtractorConfig()
	got := cfg.NumFrames()
	// 1 + 480000/320 = 1501
	want := 1501
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

	// Values should be normalised to [0, 1].
	for i, v := range out {
		if v < -1e-5 || v > 1+1e-5 {
			t.Errorf("out[%d] = %v out of [0,1]", i, v)
			break
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
