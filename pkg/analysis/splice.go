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

const spliceAnalyzerVersion = "0.1.0"

type SpliceAnalyzer struct {
	registry *zaudio.Registry
}

func NewSpliceAnalyzer(registry *zaudio.Registry) (*SpliceAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &SpliceAnalyzer{registry: registry}, nil
}

func (a *SpliceAnalyzer) Name() string {
	return "splice"
}

func (a *SpliceAnalyzer) Version() string {
	return spliceAnalyzerVersion
}

func (a *SpliceAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("splice analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze splice for %q: %w", sample.Path, err)
	}

	stats := computeSpliceStats(decoded.Buffer)
	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"splice_count":         stats.Count,
			"splice_max_strength":  stats.MaxStrength,
			"splice_mean_strength": stats.MeanStrength,
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type SpliceStats struct {
	Count        float64
	MaxStrength  float64
	MeanStrength float64
}

// computeSpliceStats detects abrupt spectral discontinuities, inspired by the
// trajectory/discontinuity perspective used in TRACE (Khan et al., 2026).
func computeSpliceStats(buffer zaudio.PCMBuffer) SpliceStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return SpliceStats{}
	}

	windowSize := 2048
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

	fft := fourier.NewFFT(windowSize)
	frame := make([]float64, windowSize)
	var (
		prev      []float64
		strengths []float64
	)
	for start := 0; start < len(mono); start += hopSize {
		clear(frame)
		end := minInt(len(mono), start+windowSize)
		copy(frame, mono[start:end])
		applyHannWindow(frame)

		mags := magnitudeSpectrum(fft.Coefficients(nil, frame))
		if prev != nil {
			var sum float64
			for i := range mags {
				diff := mags[i] - prev[i]
				sum += diff * diff
			}
			strengths = append(strengths, math.Sqrt(sum/float64(len(mags))))
		}
		prev = mags
		if end == len(mono) {
			break
		}
	}
	if len(strengths) == 0 {
		return SpliceStats{}
	}

	mean, std := meanAndStdDev(strengths)
	threshold := mean + 2*std
	var (
		count   float64
		maximum float64
		total   float64
	)
	for _, strength := range strengths {
		if strength > maximum {
			maximum = strength
		}
		if strength >= threshold {
			count++
			total += strength
		}
	}
	stats := SpliceStats{
		Count:       count,
		MaxStrength: maximum,
	}
	if count > 0 {
		stats.MeanStrength = total / count
	}
	return stats
}

func meanAndStdDev(values []float64) (mean, std float64) {
	if len(values) == 0 {
		return 0, 0
	}
	for _, value := range values {
		mean += value
	}
	mean /= float64(len(values))
	for _, value := range values {
		diff := value - mean
		std += diff * diff
	}
	std = math.Sqrt(std / float64(len(values)))
	return mean, std
}
