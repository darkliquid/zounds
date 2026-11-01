package audio_test

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestMeasureLoudnessStereo1kHzSine(t *testing.T) {
	t.Parallel()

	sampleRate := 48000
	duration := 2.0
	frames := int(duration * float64(sampleRate))
	dataMono := make([]float64, frames)
	dataStereo := make([]float64, frames*2)

	// 1 kHz sine wave with peak amplitude 1.0 (0 dBFS)
	for i := 0; i < frames; i++ {
		val := math.Sin(2 * math.Pi * 1000.0 * float64(i) / float64(sampleRate))
		dataMono[i] = val
		dataStereo[i*2] = val
		dataStereo[i*2+1] = val
	}

	bufMono := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       dataMono,
	}

	bufStereo := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   2,
		BitDepth:   16,
		Data:       dataStereo,
	}

	lufsMono, _ := zaudio.MeasureLoudness(bufMono)
	lufsStereo, truePeakStereo := zaudio.MeasureLoudness(bufStereo)

	// Per ITU-R BS.1770-4:
	// A 1 kHz full-scale sine wave in a single channel measures -3.01 LUFS (-3.01 LKFS)
	if math.Abs(lufsMono-(-3.01)) > 0.15 {
		t.Errorf("expected ~-3.01 LUFS for full-scale 1kHz mono sine, got %.3f LUFS", lufsMono)
	}

	// In stereo (both channels full-scale), loudness is +3.01 dB higher -> ~0.00 LUFS
	if math.Abs(lufsStereo-0.0) > 0.15 {
		t.Errorf("expected ~0.00 LUFS for full-scale 1kHz stereo sine, got %.3f LUFS", lufsStereo)
	}

	// Peak is 1.0 -> 0 dBTP (within 0.1 dBTP)
	if math.Abs(truePeakStereo-0.0) > 0.1 {
		t.Errorf("expected ~0.0 dBTP, got %.3f dBTP", truePeakStereo)
	}
}

func TestMeasureLoudnessSilenceGating(t *testing.T) {
	t.Parallel()

	sampleRate := 48000
	// 4 seconds of tone followed by 6 seconds of silence
	toneSeconds := 4.0
	totalSeconds := 10.0
	toneFrames := int(toneSeconds * float64(sampleRate))
	totalFrames := int(totalSeconds * float64(sampleRate))
	data := make([]float64, totalFrames)

	for i := 0; i < toneFrames; i++ {
		data[i] = math.Sin(2 * math.Pi * 1000.0 * float64(i) / float64(sampleRate))
	}

	bufWithSilence := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}

	bufToneOnly := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data[:toneFrames],
	}

	lufsWithSilence := zaudio.MeasureLUFS(bufWithSilence)
	lufsToneOnly := zaudio.MeasureLUFS(bufToneOnly)

	// BS.1770-4 gating should gate out the 6 seconds of silence!
	// Therefore, the integrated loudness should match the tone-only loudness closely (within 0.2 LUFS)
	if math.Abs(lufsWithSilence-lufsToneOnly) > 0.2 {
		t.Errorf("expected silence gating to preserve tone loudness (got withSilence=%.2f, toneOnly=%.2f)",
			lufsWithSilence, lufsToneOnly)
	}
}

func TestMeasureTruePeakCatchesInterSampleOvershoot(t *testing.T) {
	t.Parallel()

	sampleRate := 44100
	// 4 samples per cycle at 11025 Hz, phase-shifted by 45 degrees:
	// Digital samples hit sin(45 deg) = 0.7071
	// Analog true peak of this continuous sine wave is 1.0000 (+3.01 dB above 0.7071)
	frames := 1000
	data := make([]float64, frames)
	freq := 11025.0
	phase := math.Pi / 4.0 // 45 degrees

	var maxSamplePeak float64
	for i := range data {
		val := math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate) + phase)
		data[i] = val
		if math.Abs(val) > maxSamplePeak {
			maxSamplePeak = math.Abs(val)
		}
	}

	buf := zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}

	linearPeak, dbtp := zaudio.MeasureTruePeak(buf)

	// Sample peak is ~0.7071 (-3.01 dBFS)
	if math.Abs(maxSamplePeak-0.7071) > 0.01 {
		t.Errorf("expected sample peak ~0.7071, got %f", maxSamplePeak)
	}

	// True peak must reconstruct the true wave peak ~1.0000 (~0.0 dBTP)
	if math.Abs(linearPeak-1.0) > 0.05 {
		t.Errorf("expected 4x oversampled true peak ~1.0000, got %f (dbtp=%.2f)", linearPeak, dbtp)
	}
	if math.Abs(dbtp-0.0) > 0.5 {
		t.Errorf("expected true peak ~0.0 dBTP, got %.2f dBTP", dbtp)
	}
}

func TestMeasureLoudnessSilence(t *testing.T) {
	t.Parallel()

	buf := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   2,
		BitDepth:   16,
		Data:       make([]float64, 44100*2),
	}

	lufs, truePeak := zaudio.MeasureLoudness(buf)
	if lufs != -120.0 {
		t.Errorf("expected -120 LUFS for silence, got %f", lufs)
	}
	if truePeak != -120.0 {
		t.Errorf("expected -120 dBTP for silence, got %f", truePeak)
	}
}
