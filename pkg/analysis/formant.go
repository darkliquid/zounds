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

const formantAnalyzerVersion = "0.1.0"

type FormantAnalyzer struct {
	registry *zaudio.Registry
}

func NewFormantAnalyzer(registry *zaudio.Registry) (*FormantAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &FormantAnalyzer{registry: registry}, nil
}

func (a *FormantAnalyzer) Name() string {
	return "formants"
}

func (a *FormantAnalyzer) Version() string {
	return formantAnalyzerVersion
}

func (a *FormantAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("formant analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze formants for %q: %w", sample.Path, err)
	}

	stats := computeFormants(decoded.Buffer)
	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"formant_1_hz": stats.Formants[0],
			"formant_2_hz": stats.Formants[1],
			"formant_3_hz": stats.Formants[2],
			"formant_4_hz": stats.Formants[3],
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type FormantStats struct {
	Formants [4]float64
}

// computeFormants uses LPC spectral-envelope estimation after Makhoul (1975),
// "Linear Prediction: A Tutorial Review."
func computeFormants(buffer zaudio.PCMBuffer) FormantStats {
	mono := mixDownMono(buffer)
	if len(mono) < 256 || buffer.SampleRate <= 0 {
		return FormantStats{}
	}

	samples := preEmphasis(mono[:minInt(len(mono), 4096)], 0.97)
	applyHannWindow(samples)

	order := 12
	if order >= len(samples) {
		order = len(samples) - 1
	}
	if order < 2 {
		return FormantStats{}
	}

	autocorr := autocorrelation(samples, order)
	coeffs, err := levinsonDurbin(autocorr, order)
	if err != nil {
		return FormantStats{}
	}

	peaks := lpcEnvelopePeaks(coeffs, buffer.SampleRate)
	var stats FormantStats
	for i := 0; i < len(stats.Formants) && i < len(peaks); i++ {
		stats.Formants[i] = peaks[i]
	}
	return stats
}

func preEmphasis(samples []float64, coefficient float64) []float64 {
	out := append([]float64(nil), samples...)
	for i := len(out) - 1; i >= 1; i-- {
		out[i] -= coefficient * out[i-1]
	}
	return out
}

func autocorrelation(samples []float64, order int) []float64 {
	values := make([]float64, order+1)
	for lag := 0; lag <= order; lag++ {
		for i := 0; i+lag < len(samples); i++ {
			values[lag] += samples[i] * samples[i+lag]
		}
	}
	return values
}

func levinsonDurbin(autocorr []float64, order int) ([]float64, error) {
	if len(autocorr) <= order || autocorr[0] == 0 {
		return nil, fmt.Errorf("invalid autocorrelation")
	}

	coeffs := make([]float64, order+1)
	coeffs[0] = 1
	errorTerm := autocorr[0]
	for i := 1; i <= order; i++ {
		reflection := autocorr[i]
		for j := 1; j < i; j++ {
			reflection -= coeffs[j] * autocorr[i-j]
		}
		reflection /= errorTerm

		next := append([]float64(nil), coeffs...)
		next[i] = reflection
		for j := 1; j < i; j++ {
			next[j] = coeffs[j] - reflection*coeffs[i-j]
		}
		coeffs = next
		errorTerm *= 1 - reflection*reflection
		if errorTerm <= 0 {
			break
		}
	}
	return coeffs, nil
}

func lpcEnvelopePeaks(coeffs []float64, sampleRate int) []float64 {
	nyquist := float64(sampleRate) / 2
	maxFreq := math.Min(5000, nyquist)
	type peak struct {
		freq  float64
		power float64
	}
	peaks := make([]peak, 0, 8)
	values := make([]float64, 0, int(maxFreq/10))
	freqs := make([]float64, 0, int(maxFreq/10))

	for freq := 50.0; freq <= maxFreq; freq += 10 {
		omega := 2 * math.Pi * freq / float64(sampleRate)
		realPart, imagPart := 1.0, 0.0
		for k := 1; k < len(coeffs); k++ {
			realPart -= coeffs[k] * math.Cos(omega*float64(k))
			imagPart += coeffs[k] * math.Sin(omega*float64(k))
		}
		power := 1 / math.Max(1e-9, realPart*realPart+imagPart*imagPart)
		freqs = append(freqs, freq)
		values = append(values, power)
	}

	for i := 1; i < len(values)-1; i++ {
		if values[i] <= values[i-1] || values[i] < values[i+1] {
			continue
		}
		peaks = append(peaks, peak{freq: freqs[i], power: values[i]})
	}

	sort.Slice(peaks, func(i, j int) bool {
		if peaks[i].freq == peaks[j].freq {
			return peaks[i].power > peaks[j].power
		}
		return peaks[i].freq < peaks[j].freq
	})

	out := make([]float64, 0, 4)
	for _, peak := range peaks {
		if len(out) == 4 {
			break
		}
		out = append(out, peak.freq)
	}
	return out
}
