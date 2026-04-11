package analysis

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const hpssAnalyzerVersion = "0.1.0"

type HPSSAnalyzer struct {
	registry *zaudio.Registry
}

func NewHPSSAnalyzer(registry *zaudio.Registry) (*HPSSAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &HPSSAnalyzer{registry: registry}, nil
}

func (a *HPSSAnalyzer) Name() string {
	return "hpss"
}

func (a *HPSSAnalyzer) Version() string {
	return hpssAnalyzerVersion
}

func (a *HPSSAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("hpss analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze hpss for %q: %w", sample.Path, err)
	}

	stats := computeHPSSStats(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"harmonic_energy_ratio":     stats.HarmonicEnergyRatio,
			"percussive_energy_ratio":   stats.PercussiveEnergyRatio,
			"harmonic_percussive_ratio": stats.HarmonicPercussiveRatio,
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type HPSSStats struct {
	HarmonicEnergyRatio     float64
	PercussiveEnergyRatio   float64
	HarmonicPercussiveRatio float64
}

// computeHPSSStats applies Fitzgerald's median-filter HPSS on the magnitude
// spectrogram and summarizes the harmonic/percussive energy split as ratios.
func computeHPSSStats(buffer zaudio.PCMBuffer) HPSSStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return HPSSStats{}
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
	spectrogram := make([][]float64, 0, len(mono)/hopSize+1)

	for start := 0; start < len(mono); start += hopSize {
		frame := make([]float64, windowSize)
		end := start + windowSize
		if end > len(mono) {
			end = len(mono)
		}
		copy(frame, mono[start:end])
		applyHannWindow(frame)

		coeff := fft.Coefficients(nil, frame)
		spectrogram = append(spectrogram, magnitudeSpectrum(coeff))

		if end == len(mono) {
			break
		}
	}

	if len(spectrogram) == 0 {
		return HPSSStats{}
	}

	const radius = 8
	var harmonicEnergy, percussiveEnergy float64

	for t := range spectrogram {
		for f := range spectrogram[t] {
			hMedian := timeMedian(spectrogram, t, f, radius)
			pMedian := frequencyMedian(spectrogram[t], f, radius)
			denom := hMedian + pMedian
			if denom == 0 {
				continue
			}

			mag := spectrogram[t][f]
			hMask := hMedian / denom
			pMask := pMedian / denom
			harmonic := mag * hMask
			percussive := mag * pMask
			harmonicEnergy += harmonic * harmonic
			percussiveEnergy += percussive * percussive
		}
	}

	totalEnergy := harmonicEnergy + percussiveEnergy
	if totalEnergy == 0 {
		return HPSSStats{}
	}

	stats := HPSSStats{
		HarmonicEnergyRatio:   harmonicEnergy / totalEnergy,
		PercussiveEnergyRatio: percussiveEnergy / totalEnergy,
	}
	if percussiveEnergy > 0 {
		stats.HarmonicPercussiveRatio = harmonicEnergy / percussiveEnergy
	}
	return stats
}

func timeMedian(spectrogram [][]float64, frameIndex, binIndex, radius int) float64 {
	start := maxInt(0, frameIndex-radius)
	end := minInt(len(spectrogram), frameIndex+radius+1)
	values := make([]float64, 0, end-start)
	for i := start; i < end; i++ {
		values = append(values, spectrogram[i][binIndex])
	}
	return median(values)
}

func frequencyMedian(frame []float64, binIndex, radius int) float64 {
	start := maxInt(0, binIndex-radius)
	end := minInt(len(frame), binIndex+radius+1)
	values := append([]float64(nil), frame[start:end]...)
	return median(values)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
