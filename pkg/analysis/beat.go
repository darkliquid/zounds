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

const beatAnalyzerVersion = "0.2.0"

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

	metrics := map[string]float64{
		"tempo_bpm":           stats.BPM,
		"tempo_confidence":    stats.Confidence,
		"beat_period_seconds": stats.BeatPeriodSeconds,
		"onset_strength":      stats.OnsetStrength,
		"beat_count_estimate": stats.BeatCountEstimate,
	}

	attributes := map[string]string{
		"channel_layout": channelLayout(decoded.Buffer.Channels),
	}
	if stats.Confidence >= 0.50 && stats.BPM > 0 {
		attributes["tempo_bpm"] = fmt.Sprintf("%.2f", stats.BPM)
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics:     metrics,
		Attributes:  attributes,
	}, nil
}

type BeatStats struct {
	BPM               float64
	Confidence        float64
	BeatPeriodSeconds float64
	OnsetStrength     float64
	BeatCountEstimate float64
}

func computeBeatStats(buffer zaudio.PCMBuffer) BeatStats {
	mono := mixDownMono(buffer)
	if len(mono) < 1024 || buffer.SampleRate <= 0 {
		var onsetStrength float64
		if len(mono) > 0 {
			for _, s := range mono {
				onsetStrength += math.Abs(s)
			}
			onsetStrength /= float64(len(mono))
		}
		return BeatStats{OnsetStrength: onsetStrength}
	}

	const windowSize = 1024
	const hopSize = 512

	onsets := onsetEnvelope(mono, buffer.SampleRate, windowSize, hopSize)
	if len(onsets) < 2 {
		return BeatStats{}
	}

	var sumFlux float64
	for _, value := range onsets {
		sumFlux += value
	}
	onsetStrength := sumFlux / float64(len(onsets))

	Fs := float64(buffer.SampleRate) / float64(hopSize)

	// BPM search range: 50 BPM to 220 BPM
	minLag := int(math.Round(60.0 * Fs / 220.0))
	maxLag := int(math.Round(60.0 * Fs / 50.0))
	if minLag < 2 {
		minLag = 2
	}
	if maxLag >= len(onsets) {
		maxLag = len(onsets) - 1
	}
	if maxLag <= minLag {
		return BeatStats{OnsetStrength: onsetStrength}
	}

	corrs := make([]float64, maxLag+2)
	scores := make([]float64, maxLag+2)
	var sumCorr float64
	var countCorr int

	for lag := minLag; lag <= maxLag; lag++ {
		var sumXY, sumXX, sumYY float64
		M := len(onsets) - lag
		for i := 0; i < M; i++ {
			x := onsets[i+lag]
			y := onsets[i]
			sumXY += x * y
			sumXX += x * x
			sumYY += y * y
		}
		if sumXX > 0 && sumYY > 0 {
			r := sumXY / math.Sqrt(sumXX*sumYY)
			corrs[lag] = r
			sumCorr += r
			countCorr++

			// Tempo prior centered at 120 BPM with width sigma = 1.2 octaves
			bpm := 60.0 * Fs / float64(lag)
			octaveDiff := math.Log2(bpm / 120.0)
			weight := math.Exp(-0.5 * (octaveDiff * octaveDiff) / (1.2 * 1.2))

			scores[lag] = r * weight
		}
	}

	if countCorr == 0 {
		return BeatStats{OnsetStrength: onsetStrength}
	}
	meanCorr := sumCorr / float64(countCorr)

	bestLag := 0
	bestScore := -1.0
	for lag := minLag; lag <= maxLag; lag++ {
		isPeak := true
		if lag > minLag && corrs[lag] < corrs[lag-1] {
			isPeak = false
		}
		if lag < maxLag && corrs[lag] < corrs[lag+1] {
			isPeak = false
		}
		if isPeak && scores[lag] > bestScore {
			bestScore = scores[lag]
			bestLag = lag
		}
	}

	if bestLag == 0 {
		for lag := minLag; lag <= maxLag; lag++ {
			if scores[lag] > bestScore {
				bestScore = scores[lag]
				bestLag = lag
			}
		}
	}

	if bestLag < minLag || bestLag > maxLag {
		return BeatStats{OnsetStrength: onsetStrength}
	}

	// Parabolic lag interpolation
	delta := 0.0
	if bestLag > minLag && bestLag < maxLag {
		y1 := scores[bestLag-1]
		y2 := scores[bestLag]
		y3 := scores[bestLag+1]
		denom := 2.0 * (y1 - 2.0*y2 + y3)
		if math.Abs(denom) > 1e-12 {
			delta = (y1 - y3) / denom
			delta = math.Max(-0.5, math.Min(0.5, delta))
		}
	}

	lagInterp := float64(bestLag) + delta
	bpm := 60.0 * Fs / lagInterp
	beatPeriod := 60.0 / bpm

	bestCorr := corrs[bestLag]
	prominence := 1.0
	if meanCorr > 1e-6 {
		prominence = bestCorr / meanCorr
	}

	onsetCount := countEnvelopePeaks(onsets, 0.20)
	duration := buffer.Duration().Seconds()

	conf := bestCorr * math.Min(1.0, prominence/2.0)
	if duration < 1.2 || onsetCount < 3 {
		conf *= 0.15
	} else if onsetCount < 4 {
		conf *= 0.5
	}

	confidence := math.Max(0.0, math.Min(1.0, conf))

	const confidenceThreshold = 0.50
	if confidence < confidenceThreshold {
		return BeatStats{
			Confidence:    confidence,
			OnsetStrength: onsetStrength,
		}
	}

	beatCount := 0.0
	if beatPeriod > 0 {
		beatCount = duration / beatPeriod
	}

	return BeatStats{
		BPM:               bpm,
		Confidence:        confidence,
		BeatPeriodSeconds: beatPeriod,
		OnsetStrength:     onsetStrength,
		BeatCountEstimate: beatCount,
	}
}

func countEnvelopePeaks(envelope []float64, minHeightRatio float64) int {
	var maxVal float64
	for _, v := range envelope {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 1e-9 {
		return 0
	}
	threshold := maxVal * minHeightRatio

	count := 0
	for i := 1; i < len(envelope)-1; i++ {
		if envelope[i] > threshold && envelope[i] >= envelope[i-1] && envelope[i] >= envelope[i+1] {
			count++
			i += 2
		}
	}
	return count
}

func onsetEnvelope(samples []float64, sampleRate, windowSize, hopSize int) []float64 {
	if len(samples) < 2 || windowSize <= 0 || sampleRate <= 0 {
		return nil
	}

	fft := fourier.NewFFT(windowSize)
	frame := make([]float64, windowSize)
	power := make([]float64, windowSize/2+1)

	const melBands = 40
	filters := melFilterBank(melBands, windowSize, sampleRate)
	currentMel := make([]float64, melBands)
	prevLogMel := make([]float64, melBands)
	currLogMel := make([]float64, melBands)

	var envelope []float64
	hasPrev := false

	for start := 0; start < len(samples); start += hopSize {
		clear(frame)
		end := start + windowSize
		if end > len(samples) {
			end = len(samples)
		}
		copy(frame, samples[start:end])
		applyHannWindow(frame)

		coeff := fft.Coefficients(nil, frame)
		powerSpectrumInto(coeff, power)

		applyFilterBankInto(power, filters, currentMel)

		// Log-compression: log(1 + 100 * E)
		for i := range currentMel {
			currLogMel[i] = math.Log1p(100.0 * currentMel[i])
		}

		if hasPrev {
			var flux float64
			for i := 0; i < melBands; i++ {
				// SuperFlux frequency-domain max filter: max(prev[i-1], prev[i], prev[i+1])
				ref := prevLogMel[i]
				if i > 0 && prevLogMel[i-1] > ref {
					ref = prevLogMel[i-1]
				}
				if i+1 < melBands && prevLogMel[i+1] > ref {
					ref = prevLogMel[i+1]
				}

				diff := currLogMel[i] - ref
				if diff > 0 {
					flux += diff
				}
			}
			envelope = append(envelope, flux/float64(melBands))
		}
		copy(prevLogMel, currLogMel)
		hasPrev = true

		if end == len(samples) {
			break
		}
	}

	return normalizeEnvelope(envelope)
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
