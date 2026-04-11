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
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return PitchStats{}
	}

	segment := strongestWindow(mono, 4096, 1024)
	if len(segment) < 2 {
		return PitchStats{}
	}

	minLag := int(float64(buffer.SampleRate) / 2000.0)
	maxLag := int(float64(buffer.SampleRate) / 50.0)
	if minLag < 1 {
		minLag = 1
	}
	if maxLag >= len(segment) {
		maxLag = len(segment) - 1
	}

	bestLag := 0
	bestScore := 0.0
	scores := make([]float64, maxLag+1)
	for lag := minLag; lag <= maxLag; lag++ {
		score := normalizedAutocorrelation(segment, lag)
		scores[lag] = score
		if score > bestScore {
			bestScore = score
			bestLag = lag
		}
	}

	threshold := bestScore * 0.9
	for lag := minLag + 1; lag < maxLag; lag++ {
		score := scores[lag]
		if score < threshold {
			continue
		}
		if score >= scores[lag-1] && score >= scores[lag+1] {
			bestLag = lag
			bestScore = score
			break
		}
	}

	if bestLag == 0 || bestScore <= 0 {
		return PitchStats{}
	}

	frequency := float64(buffer.SampleRate) / float64(bestLag)
	midi := 69.0 + 12.0*math.Log2(frequency/440.0)
	rounded := math.Round(midi)
	cents := (midi - rounded) * 100

	return PitchStats{
		FrequencyHz: frequency,
		MIDINote:    midi,
		Confidence:  bestScore,
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
