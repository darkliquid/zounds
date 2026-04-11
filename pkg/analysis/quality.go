package analysis

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const qualityAnalyzerVersion = "0.1.0"

type QualityAnalyzer struct {
	registry *zaudio.Registry
}

func NewQualityAnalyzer(registry *zaudio.Registry) (*QualityAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &QualityAnalyzer{registry: registry}, nil
}

func (a *QualityAnalyzer) Name() string {
	return "quality"
}

func (a *QualityAnalyzer) Version() string {
	return qualityAnalyzerVersion
}

func (a *QualityAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("quality analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze quality for %q: %w", sample.Path, err)
	}

	stats := computeQuality(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"clipping_ratio":   stats.ClippingRatio,
			"dc_offset":        stats.DCOffset,
			"silence_ratio":    stats.SilenceRatio,
			"estimated_snr_db": stats.EstimatedSNRDB,
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type QualityStats struct {
	ClippingRatio  float64
	DCOffset       float64
	SilenceRatio   float64
	EstimatedSNRDB float64
}

// computeQuality uses simple curation-oriented signal diagnostics aligned with
// standard loudness/peak practice from ITU-R BS.1770 and EBU R128.
func computeQuality(buffer zaudio.PCMBuffer) QualityStats {
	mono := mixDownMono(buffer)
	if len(mono) == 0 {
		return QualityStats{}
	}

	var (
		sum          float64
		clippedCount int
		silentCount  int
	)
	for _, sample := range mono {
		sum += sample
		if math.Abs(sample) >= 0.999 {
			clippedCount++
		}
		if math.Abs(sample) <= 0.001 {
			silentCount++
		}
	}

	windows := qualityWindowRMS(mono, 1024)
	noiseFloor := firstPositiveSorted(windows, 0.1)
	signalLevel := firstPositiveSorted(windows, 0.9)
	estimatedSNRDB := 0.0
	if noiseFloor > 0 && signalLevel > 0 {
		estimatedSNRDB = 20 * math.Log10(signalLevel/noiseFloor)
	}

	return QualityStats{
		ClippingRatio:  float64(clippedCount) / float64(len(mono)),
		DCOffset:       sum / float64(len(mono)),
		SilenceRatio:   float64(silentCount) / float64(len(mono)),
		EstimatedSNRDB: estimatedSNRDB,
	}
}

func qualityWindowRMS(samples []float64, windowSize int) []float64 {
	if len(samples) == 0 || windowSize <= 0 {
		return nil
	}
	values := make([]float64, 0, len(samples)/windowSize+1)
	for start := 0; start < len(samples); start += windowSize {
		end := minInt(len(samples), start+windowSize)
		var sumSq float64
		for _, sample := range samples[start:end] {
			sumSq += sample * sample
		}
		values = append(values, math.Sqrt(sumSq/float64(end-start)))
	}
	return values
}

func firstPositiveSorted(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	value := percentile(sorted, p)
	if value > 0 {
		return value
	}
	return firstPositive(sorted)
}
