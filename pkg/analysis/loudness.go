package analysis

import (
	"context"
	"fmt"
	"math"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const loudnessAnalyzerVersion = "0.1.0"

type LoudnessAnalyzer struct {
	registry *zaudio.Registry
}

func NewLoudnessAnalyzer(registry *zaudio.Registry) (*LoudnessAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &LoudnessAnalyzer{registry: registry}, nil
}

func (a *LoudnessAnalyzer) Name() string {
	return "loudness"
}

func (a *LoudnessAnalyzer) Version() string {
	return loudnessAnalyzerVersion
}

func (a *LoudnessAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("loudness analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze loudness for %q: %w", sample.Path, err)
	}

	stats := computeLoudness(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"peak":             stats.Peak,
			"peak_dbfs":        stats.PeakDBFS,
			"rms":              stats.RMS,
			"rms_dbfs":         stats.RMSDBFS,
			"crest_factor":     stats.CrestFactor,
			"integrated_lufs":  stats.IntegratedLUFS,
			"mean_abs":         stats.MeanAbsolute,
			"duration_seconds": decoded.Buffer.Duration().Seconds(),
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type LoudnessStats struct {
	Peak           float64
	PeakDBFS       float64
	RMS            float64
	RMSDBFS        float64
	CrestFactor    float64
	IntegratedLUFS float64
	MeanAbsolute   float64
}

func computeLoudness(buffer zaudio.PCMBuffer) LoudnessStats {
	if len(buffer.Data) == 0 {
		return LoudnessStats{}
	}

	var (
		peak   float64
		sumSq  float64
		sumAbs float64
	)

	for _, sample := range buffer.Data {
		abs := math.Abs(sample)
		if abs > peak {
			peak = abs
		}
		sumSq += sample * sample
		sumAbs += abs
	}

	rms := math.Sqrt(sumSq / float64(len(buffer.Data)))
	peakDBFS := linearToDBFS(peak)
	rmsDBFS := linearToDBFS(rms)

	crest := 0.0
	if rms > 0 {
		crest = peak / rms
	}

	return LoudnessStats{
		Peak:           peak,
		PeakDBFS:       peakDBFS,
		RMS:            rms,
		RMSDBFS:        rmsDBFS,
		CrestFactor:    crest,
		IntegratedLUFS: rmsDBFS,
		MeanAbsolute:   sumAbs / float64(len(buffer.Data)),
	}
}

func linearToDBFS(value float64) float64 {
	if value <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(value)
}
