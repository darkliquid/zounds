package analysis

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const harmonicsAnalyzerVersion = "0.1.0"

type HarmonicsAnalyzer struct {
	registry *zaudio.Registry
}

func NewHarmonicsAnalyzer(registry *zaudio.Registry) (*HarmonicsAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &HarmonicsAnalyzer{registry: registry}, nil
}

func (a *HarmonicsAnalyzer) Name() string {
	return "harmonics"
}

func (a *HarmonicsAnalyzer) Version() string {
	return harmonicsAnalyzerVersion
}

func (a *HarmonicsAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("harmonics analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze harmonics for %q: %w", sample.Path, err)
	}

	stats := computeHarmonics(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"harmonic_ratio": stats.HarmonicRatio,
			"inharmonicity":  stats.Inharmonicity,
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type HarmonicsStats struct {
	HarmonicRatio float64
	Inharmonicity float64
}

// computeHarmonics measures harmonic alignment by comparing dominant spectral
// peaks with integer multiples of the estimated fundamental frequency.
func computeHarmonics(buffer zaudio.PCMBuffer) HarmonicsStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return HarmonicsStats{}
	}

	windowSize := 4096
	if len(mono) < windowSize {
		windowSize = nextPowerOfTwo(len(mono))
		if windowSize < 2 {
			windowSize = len(mono)
		}
	}

	frame := make([]float64, windowSize)
	copy(frame, mono[:minInt(len(mono), windowSize)])
	applyHannWindow(frame)

	fft := fourier.NewFFT(windowSize)
	mags := magnitudeSpectrum(fft.Coefficients(nil, frame))
	peaks := spectralPeaks(mags, buffer.SampleRate, fft)
	fundamental := estimateFundamental(peaks)
	if fundamental <= 0 {
		fundamental = dominantFrequency(mags, buffer.SampleRate, fft)
	}
	if fundamental <= 0 {
		return HarmonicsStats{}
	}

	binWidth := float64(buffer.SampleRate) / float64(windowSize)
	tolerance := math.Max(binWidth, fundamental*0.015)
	var (
		totalEnergy       float64
		harmonicEnergy    float64
		weightedDeviation float64
		deviationWeight   float64
	)

	for _, peak := range peaks {
		freq := peak.Frequency
		energy := peak.Energy
		totalEnergy += energy
		if freq < fundamental*0.5 {
			continue
		}

		harmonicNumber := math.Max(1, math.Round(freq/fundamental))
		expected := harmonicNumber * fundamental
		diff := math.Abs(freq - expected)
		if diff <= tolerance {
			harmonicEnergy += energy
		}
		weightedDeviation += energy * (diff / expected)
		deviationWeight += energy
	}

	stats := HarmonicsStats{}
	if totalEnergy > 0 {
		stats.HarmonicRatio = harmonicEnergy / totalEnergy
	}
	if deviationWeight > 0 {
		stats.Inharmonicity = weightedDeviation / deviationWeight
	}
	return stats
}

type spectralPeak struct {
	Frequency float64
	Energy    float64
}

func spectralPeaks(mags []float64, sampleRate int, fft *fourier.FFT) []spectralPeak {
	peaks := make([]spectralPeak, 0, 16)
	for i := 1; i < len(mags)-1; i++ {
		if mags[i] <= mags[i-1] || mags[i] < mags[i+1] {
			continue
		}
		energy := mags[i] * mags[i]
		peaks = append(peaks, spectralPeak{
			Frequency: fft.Freq(i) * float64(sampleRate),
			Energy:    energy,
		})
	}

	sort.Slice(peaks, func(i, j int) bool {
		return peaks[i].Energy > peaks[j].Energy
	})
	if len(peaks) > 12 {
		peaks = peaks[:12]
	}
	return peaks
}

func estimateFundamental(peaks []spectralPeak) float64 {
	if len(peaks) == 0 {
		return 0
	}
	threshold := peaks[0].Energy * 0.2
	fundamental := 0.0
	for _, peak := range peaks {
		if peak.Energy < threshold || peak.Frequency < 50 {
			continue
		}
		if fundamental == 0 || peak.Frequency < fundamental {
			fundamental = peak.Frequency
		}
	}
	return fundamental
}
