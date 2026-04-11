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

const dynamicsAnalyzerVersion = "0.1.0"

type DynamicsAnalyzer struct {
	registry *zaudio.Registry
}

func NewDynamicsAnalyzer(registry *zaudio.Registry) (*DynamicsAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &DynamicsAnalyzer{registry: registry}, nil
}

func (a *DynamicsAnalyzer) Name() string {
	return "dynamics"
}

func (a *DynamicsAnalyzer) Version() string {
	return dynamicsAnalyzerVersion
}

func (a *DynamicsAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("dynamics analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze dynamics for %q: %w", sample.Path, err)
	}

	stats := computeDynamics(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"dynamic_range_db": stats.DynamicRangeDB,
			"attack_sharpness": stats.AttackSharpness,
			"sustain_ratio":    stats.SustainRatio,
			"transient_rate":   stats.TransientRate,
			"window_count":     float64(stats.WindowCount),
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type DynamicsStats struct {
	DynamicRangeDB  float64
	AttackSharpness float64
	SustainRatio    float64
	TransientRate   float64
	WindowCount     int
}

func computeDynamics(buffer zaudio.PCMBuffer) DynamicsStats {
	if len(buffer.Data) == 0 {
		return DynamicsStats{}
	}

	const windowSize = 1024
	windows := make([]float64, 0, len(buffer.Data)/windowSize+1)

	var (
		maxDelta       float64
		transientCount int
		previousSample float64
		havePrevSample bool
	)

	for start := 0; start < len(buffer.Data); start += windowSize {
		end := start + windowSize
		if end > len(buffer.Data) {
			end = len(buffer.Data)
		}

		var sumSq float64
		for _, sample := range buffer.Data[start:end] {
			sumSq += sample * sample

			if havePrevSample {
				delta := math.Abs(sample - previousSample)
				if delta > maxDelta {
					maxDelta = delta
				}
				if delta >= 0.2 {
					transientCount++
				}
			}
			previousSample = sample
			havePrevSample = true
		}

		windowRMS := math.Sqrt(sumSq / float64(end-start))
		windows = append(windows, windowRMS)
	}

	sorted := append([]float64(nil), windows...)
	sort.Float64s(sorted)

	low := firstPositive(sorted)
	if low == 0 {
		low = percentile(sorted, 0.05)
	}
	high := percentile(sorted, 0.95)
	dynamicRange := 0.0
	if low > 0 && high > 0 {
		dynamicRange = 20 * math.Log10(high/low)
	}

	median := percentile(sorted, 0.5)
	sustainRatio := 0.0
	if high > 0 {
		sustainRatio = median / high
	}

	durationSeconds := buffer.Duration().Seconds()
	transientRate := 0.0
	if durationSeconds > 0 {
		transientRate = float64(transientCount) / durationSeconds
	}

	return DynamicsStats{
		DynamicRangeDB:  dynamicRange,
		AttackSharpness: maxDelta,
		SustainRatio:    sustainRatio,
		TransientRate:   transientRate,
		WindowCount:     len(windows),
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	if lower == upper {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func firstPositive(sorted []float64) float64 {
	for _, value := range sorted {
		if value > 0 {
			return value
		}
	}
	return 0
}
