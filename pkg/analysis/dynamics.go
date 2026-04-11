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

const dynamicsAnalyzerVersion = "0.3.0"

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
			"dynamic_range_db":   stats.DynamicRangeDB,
			"attack_sharpness":   stats.AttackSharpness,
			"sustain_ratio":      stats.SustainRatio,
			"transient_rate":     stats.TransientRate,
			"temporal_centroid":  stats.TemporalCentroid,
			"attack_time_ms":     stats.AttackTimeMs,
			"decay_time_ms":      stats.DecayTimeMs,
			"release_time_ms":    stats.ReleaseTimeMs,
			"sustain_level_dbfs": stats.SustainLevelDBFS,
			"window_count":       float64(stats.WindowCount),
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type DynamicsStats struct {
	DynamicRangeDB   float64
	AttackSharpness  float64
	SustainRatio     float64
	TransientRate    float64
	TemporalCentroid float64
	AttackTimeMs     float64
	DecayTimeMs      float64
	ReleaseTimeMs    float64
	SustainLevelDBFS float64
	WindowCount      int
}

// computeDynamics includes a temporal centroid over the RMS envelope following
// common temporal descriptor practice summarized by Peeters (2004), "A large
// set of audio features for sound description."
func computeDynamics(buffer zaudio.PCMBuffer) DynamicsStats {
	mono := mixDownMono(buffer)
	if len(mono) == 0 || buffer.SampleRate <= 0 {
		return DynamicsStats{}
	}

	const windowSize = 1024
	windows := make([]float64, 0, len(mono)/windowSize+1)

	var (
		maxDelta       float64
		transientCount int
		previousSample float64
		havePrevSample bool
	)

	for start := 0; start < len(mono); start += windowSize {
		end := start + windowSize
		if end > len(mono) {
			end = len(mono)
		}

		var sumSq float64
		for _, sample := range mono[start:end] {
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

	temporalCentroid := normalizedTemporalCentroid(windows)
	attackTimeMs, decayTimeMs, releaseTimeMs, sustainLevelDBFS := adsrFromEnvelope(windows, buffer.SampleRate, windowSize)

	durationSeconds := buffer.Duration().Seconds()
	transientRate := 0.0
	if durationSeconds > 0 {
		transientRate = float64(transientCount) / durationSeconds
	}

	return DynamicsStats{
		DynamicRangeDB:   dynamicRange,
		AttackSharpness:  maxDelta,
		SustainRatio:     sustainRatio,
		TransientRate:    transientRate,
		TemporalCentroid: temporalCentroid,
		AttackTimeMs:     attackTimeMs,
		DecayTimeMs:      decayTimeMs,
		ReleaseTimeMs:    releaseTimeMs,
		SustainLevelDBFS: sustainLevelDBFS,
		WindowCount:      len(windows),
	}
}

func normalizedTemporalCentroid(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var (
		total    float64
		weighted float64
		denom    = math.Max(1, float64(len(values)-1))
	)
	for i, value := range values {
		if value <= 0 {
			continue
		}
		position := float64(i) / denom
		total += value
		weighted += position * value
	}
	if total == 0 {
		return 0
	}
	return weighted / total
}

func adsrFromEnvelope(values []float64, sampleRate, windowSize int) (attackMs, decayMs, releaseMs, sustainDBFS float64) {
	if len(values) == 0 || sampleRate <= 0 || windowSize <= 0 {
		return 0, 0, 0, 0
	}

	peak := 0.0
	peakIndex := 0
	for i, value := range values {
		if value > peak {
			peak = value
			peakIndex = i
		}
	}
	if peak == 0 {
		return 0, 0, 0, 0
	}

	normalized := make([]float64, len(values))
	for i, value := range values {
		normalized[i] = value / peak
	}

	onsetIndex := 0
	for i, value := range normalized {
		if value >= 0.1 {
			onsetIndex = i
			break
		}
	}

	activeEnd := len(normalized) - 1
	for activeEnd > peakIndex && normalized[activeEnd] < 0.05 {
		activeEnd--
	}
	if activeEnd < peakIndex {
		activeEnd = peakIndex
	}

	sustainWindowStart := peakIndex
	if activeEnd > peakIndex {
		sustainWindowStart = peakIndex + (activeEnd-peakIndex)/2
	}
	sustainSlice := normalized[sustainWindowStart : activeEnd+1]
	sustainLevel := percentile(append([]float64(nil), sustainSlice...), 0.5)
	if sustainLevel <= 0 {
		sustainLevel = percentile(append([]float64(nil), normalized[peakIndex:activeEnd+1]...), 0.25)
	}

	sustainStart := peakIndex
	for i := peakIndex; i <= activeEnd; i++ {
		if normalized[i] <= sustainLevel*1.1 {
			sustainStart = i
			break
		}
	}

	releaseStart := activeEnd
	for i := activeEnd; i >= sustainStart; i-- {
		if normalized[i] >= sustainLevel*0.9 {
			releaseStart = i
			break
		}
	}

	windowMs := 1000 * float64(windowSize) / float64(sampleRate)
	attackMs = float64(maxInt(0, peakIndex-onsetIndex)) * windowMs
	decayMs = float64(maxInt(0, sustainStart-peakIndex)) * windowMs
	releaseMs = float64(maxInt(0, activeEnd-releaseStart)) * windowMs
	sustainDBFS = amplitudeToDBFS(sustainLevel)
	return attackMs, decayMs, releaseMs, sustainDBFS
}

func amplitudeToDBFS(value float64) float64 {
	if value <= 0 {
		return -120
	}
	return 20 * math.Log10(value)
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
