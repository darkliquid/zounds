package analysis

import (
	"context"
	"fmt"
	"math"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const beatAnalyzerVersion = "0.1.0"

type BeatAnalyzer struct {
	registry *zaudio.Registry
}

func NewBeatAnalyzer(registry *zaudio.Registry) (*BeatAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &BeatAnalyzer{registry: registry}, nil
}

func (a *BeatAnalyzer) Name() string {
	return "beat"
}

func (a *BeatAnalyzer) Version() string {
	return beatAnalyzerVersion
}

func (a *BeatAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("beat analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze beat for %q: %w", sample.Path, err)
	}

	stats := computeBeatStats(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"tempo_bpm":           stats.BPM,
			"beat_period_seconds": stats.BeatPeriodSeconds,
			"onset_strength":      stats.OnsetStrength,
			"beat_count_estimate": stats.BeatCountEstimate,
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type BeatStats struct {
	BPM               float64
	BeatPeriodSeconds float64
	OnsetStrength     float64
	BeatCountEstimate float64
}

func computeBeatStats(buffer zaudio.PCMBuffer) BeatStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return BeatStats{}
	}

	windowSize := 1024
	if len(mono) < windowSize {
		windowSize = nextPowerOfTwo(len(mono))
		if windowSize < 2 {
			windowSize = len(mono)
		}
	}
	hopSize := windowSize / 2
	if hopSize == 0 {
		hopSize = 1
	}

	onsets := onsetEnvelope(mono, windowSize, hopSize)
	if len(onsets) < 2 {
		return BeatStats{}
	}

	lag := strongestTempoLag(onsets, buffer.SampleRate, hopSize, 60, 200)
	if lag == 0 {
		return BeatStats{}
	}

	bpm := 60 * float64(buffer.SampleRate) / float64(hopSize*lag)
	beatPeriod := 60.0 / bpm

	var onsetStrength float64
	for _, value := range onsets {
		onsetStrength += value
	}
	onsetStrength /= float64(len(onsets))

	duration := buffer.Duration().Seconds()
	beatCount := 0.0
	if beatPeriod > 0 {
		beatCount = duration / beatPeriod
	}

	return BeatStats{
		BPM:               bpm,
		BeatPeriodSeconds: beatPeriod,
		OnsetStrength:     onsetStrength,
		BeatCountEstimate: beatCount,
	}
}

func onsetEnvelope(samples []float64, windowSize, hopSize int) []float64 {
	if len(samples) < 2 || windowSize <= 0 {
		return nil
	}

	fft := fourier.NewFFT(windowSize)
	var (
		prev     []float64
		envelope []float64
	)

	for start := 0; start < len(samples); start += hopSize {
		frame := make([]float64, windowSize)
		end := start + windowSize
		if end > len(samples) {
			end = len(samples)
		}
		copy(frame, samples[start:end])
		applyHannWindow(frame)

		coeff := fft.Coefficients(nil, frame)
		mags := magnitudeSpectrum(coeff)
		if prev != nil {
			var flux float64
			for i := range mags {
				diff := mags[i] - prev[i]
				if diff > 0 {
					flux += diff
				}
			}
			envelope = append(envelope, flux/float64(len(mags)))
		}
		prev = mags

		if end == len(samples) {
			break
		}
	}

	return normalizeEnvelope(envelope)
}

func strongestTempoLag(envelope []float64, sampleRate, hopSize, minBPM, maxBPM int) int {
	if len(envelope) < 2 {
		return 0
	}

	minLag := int(math.Round(60 * float64(sampleRate) / float64(hopSize*maxBPM)))
	maxLag := int(math.Round(60 * float64(sampleRate) / float64(hopSize*minBPM)))
	if minLag < 1 {
		minLag = 1
	}
	if maxLag >= len(envelope) {
		maxLag = len(envelope) - 1
	}

	bestLag := 0
	bestScore := 0.0
	for lag := minLag; lag <= maxLag; lag++ {
		var score float64
		for i := lag; i < len(envelope); i++ {
			score += envelope[i] * envelope[i-lag]
		}
		if score > bestScore {
			bestScore = score
			bestLag = lag
		}
	}

	return bestLag
}

func normalizeEnvelope(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}

	max := 0.0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	if max == 0 {
		return values
	}

	normalized := make([]float64, len(values))
	for i, value := range values {
		normalized[i] = value / max
	}
	return normalized
}
