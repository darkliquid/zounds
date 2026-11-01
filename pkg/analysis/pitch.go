package analysis

import (
	"context"
	"fmt"
	"math"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const pitchAnalyzerVersion = "0.1.0"

type PitchAnalyzer struct {
	registry *zaudio.Registry
}

func NewPitchAnalyzer(registry *zaudio.Registry) (*PitchAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &PitchAnalyzer{registry: registry}, nil
}

func (a *PitchAnalyzer) Name() string {
	return "pitch"
}

func (a *PitchAnalyzer) Version() string {
	return pitchAnalyzerVersion
}

func (a *PitchAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("pitch analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze pitch for %q: %w", sample.Path, err)
	}

	stats := computePitch(decoded.Buffer)

	attributes := map[string]string{
		"channel_layout": channelLayout(decoded.Buffer.Channels),
	}
	if stats.NoteName != "" {
		attributes["note_name"] = stats.NoteName
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"frequency_hz":    stats.FrequencyHz,
			"midi_note":       stats.MIDINote,
			"confidence":      stats.Confidence,
			"cents_from_note": stats.CentsOffset,
		},
		Attributes: attributes,
	}, nil
}

type PitchStats struct {
	FrequencyHz float64
	MIDINote    float64
	Confidence  float64
	CentsOffset float64
	NoteName    string
}

func computePitch(buffer zaudio.PCMBuffer) PitchStats {
	mono := mixDownMono(buffer)
	if len(mono) < 64 || buffer.SampleRate <= 0 {
		return PitchStats{}
	}

	// Check for silence
	var totalEnergy float64
	for _, s := range mono {
		totalEnergy += s * s
	}
	if totalEnergy < 1e-9 {
		return PitchStats{}
	}

	// Pitch range: 24 Hz (covers 25 Hz sub-bass / 808s) to 4200 Hz (covers 4000 Hz)
	maxLag := int(float64(buffer.SampleRate) / 24.0)
	minLag := int(float64(buffer.SampleRate) / 4200.0)
	if minLag < 2 {
		minLag = 2
	}

	// Integration window W: ideally ~2048 samples (or adapted for shorter audio)
	desiredW := 2048
	neededLen := desiredW + maxLag

	segment := mono
	if len(mono) > neededLen {
		segment = strongestWindow(mono, neededLen, neededLen/4)
	}

	W := desiredW
	if len(segment) < W+maxLag {
		W = len(segment) / 2
		if W < 16 {
			return PitchStats{}
		}
		maxLag = len(segment) - W
	}
	if maxLag <= minLag {
		return PitchStats{}
	}

	// YIN pitch estimation algorithm
	d := make([]float64, maxLag+2)
	dPrime := make([]float64, maxLag+2)
	dPrime[0] = 1.0

	var runningSum float64
	bestLag := 0
	tauFound := false
	const dipThreshold = 0.15

	segA := segment[:W]
	for tau := 1; tau <= maxLag; tau++ {
		segB := segment[tau : tau+W]
		var sum float64
		for j := range segA {
			diff := segA[j] - segB[j]
			sum += diff * diff
		}
		d[tau] = sum
		runningSum += sum

		if runningSum == 0 {
			dPrime[tau] = 1.0
		} else {
			dPrime[tau] = d[tau] / (runningSum / float64(tau))
		}

		if tau >= minLag {
			if !tauFound && dPrime[tau] < dipThreshold {
				tauFound = true
			}
			if tauFound && tau > minLag && dPrime[tau] > dPrime[tau-1] {
				bestLag = tau - 1
				break
			}
		}
	}

	// If no dip was below dipThreshold, find global minimum across valid lags
	if bestLag == 0 {
		minVal := math.MaxFloat64
		for t := minLag; t <= maxLag; t++ {
			if dPrime[t] < minVal {
				minVal = dPrime[t]
				bestLag = t
			}
		}
	}

	if bestLag < minLag || bestLag > maxLag {
		return PitchStats{}
	}

	// Parabolic lag interpolation to eliminate integer-lag quantization error
	delta := 0.0
	y2 := dPrime[bestLag]
	yMin := y2

	if bestLag > minLag && bestLag < maxLag && dPrime[bestLag+1] > 0 {
		y1 := dPrime[bestLag-1]
		y3 := dPrime[bestLag+1]
		denom := 2.0 * (y1 - 2.0*y2 + y3)
		if math.Abs(denom) > 1e-12 {
			delta = (y1 - y3) / denom
			delta = math.Max(-0.5, math.Min(0.5, delta))
			yMin = y2 - (y1-y3)*delta/4.0
		}
	}

	if yMin < 0 {
		yMin = 0
	}
	confidence := math.Max(0.0, math.Min(1.0, 1.0-yMin))

	const confidenceThreshold = 0.60
	if confidence < confidenceThreshold {
		return PitchStats{
			Confidence: confidence,
		}
	}

	tauInterp := float64(bestLag) + delta
	if tauInterp <= 0 {
		return PitchStats{Confidence: confidence}
	}

	frequency := float64(buffer.SampleRate) / tauInterp
	if frequency < 20.0 || frequency > 5000.0 {
		return PitchStats{Confidence: confidence}
	}

	midi := 69.0 + 12.0*math.Log2(frequency/440.0)
	rounded := math.Round(midi)
	cents := (midi - rounded) * 100.0

	return PitchStats{
		FrequencyHz: frequency,
		MIDINote:    midi,
		Confidence:  confidence,
		CentsOffset: cents,
		NoteName:    midiNoteName(int(rounded)),
	}
}

func strongestWindow(samples []float64, windowSize, hopSize int) []float64 {
	if len(samples) <= windowSize {
		return append([]float64(nil), samples...)
	}

	bestStart := 0
	bestEnergy := -1.0

	for start := 0; start+windowSize <= len(samples); start += hopSize {
		var energy float64
		for _, sample := range samples[start : start+windowSize] {
			energy += sample * sample
		}
		if energy > bestEnergy {
			bestEnergy = energy
			bestStart = start
		}
	}

	return append([]float64(nil), samples[bestStart:bestStart+windowSize]...)
}

func normalizedAutocorrelation(samples []float64, lag int) float64 {
	var (
		numerator float64
		energyA   float64
		energyB   float64
	)

	for i := 0; i+lag < len(samples); i++ {
		a := samples[i]
		b := samples[i+lag]
		numerator += a * b
		energyA += a * a
		energyB += b * b
	}

	if energyA == 0 || energyB == 0 {
		return 0
	}

	return numerator / math.Sqrt(energyA*energyB)
}

func midiNoteName(midi int) string {
	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	if midi < 0 {
		return ""
	}
	octave := midi/12 - 1
	return fmt.Sprintf("%s%d", noteNames[midi%12], octave)
}
