package analysis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zconvert "github.com/darkliquid/zounds/pkg/convert"
	"github.com/darkliquid/zounds/pkg/core"
)

const perceptualHashAnalyzerVersion = "0.1.0"

type PerceptualHashAnalyzer struct {
	registry *zaudio.Registry
}

type PerceptualHash struct {
	Algorithm string
	Bits      int
	Value     string
}

func NewPerceptualHashAnalyzer(registry *zaudio.Registry) (*PerceptualHashAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &PerceptualHashAnalyzer{registry: registry}, nil
}

func (a *PerceptualHashAnalyzer) Name() string {
	return "perceptual_hash"
}

func (a *PerceptualHashAnalyzer) Version() string {
	return perceptualHashAnalyzerVersion
}

func (a *PerceptualHashAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("perceptual hash analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze perceptual hash for %q: %w", sample.Path, err)
	}

	hash, err := computePerceptualHash(decoded.Buffer)
	if err != nil {
		return core.AnalysisResult{}, err
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"hash_bits": float64(hash.Bits),
		},
		Attributes: map[string]string{
			"perceptual_hash": hash.Value,
			"hash_algorithm":  hash.Algorithm,
		},
	}, nil
}

func HammingDistanceHex(left, right string) (int, error) {
	leftValue, err := strconv.ParseUint(left, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parse left hash: %w", err)
	}
	rightValue, err := strconv.ParseUint(right, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parse right hash: %w", err)
	}

	diff := leftValue ^ rightValue
	distance := 0
	for diff > 0 {
		distance += int(diff & 1)
		diff >>= 1
	}
	return distance, nil
}

func computePerceptualHash(buffer zaudio.PCMBuffer) (PerceptualHash, error) {
	mono := zaudio.PCMBuffer{
		SampleRate: buffer.SampleRate,
		Channels:   1,
		BitDepth:   buffer.BitDepth,
		Data:       mixDownMono(buffer),
	}
	if len(mono.Data) < 16 || mono.SampleRate <= 0 {
		return PerceptualHash{
			Algorithm: "spectral-8x8",
			Bits:      64,
			Value:     "0000000000000000",
		}, nil
	}

	if mono.SampleRate != 8000 {
		resampled, err := zconvert.ResampleLinear(mono, zconvert.ResampleOptions{TargetSampleRate: 8000})
		if err != nil {
			return PerceptualHash{}, fmt.Errorf("compute perceptual hash: %w", err)
		}
		mono = resampled
	}

	const windows = 8
	const bands = windows * windows

	bandEnergies := perceptualBands(mono.Data, bands)
	var meanEnergy float64
	for _, energy := range bandEnergies {
		meanEnergy += energy
	}
	meanEnergy /= float64(len(bandEnergies))

	var bits uint64
	for band, energy := range bandEnergies {
		if energy >= meanEnergy {
			bits |= 1 << uint(band)
		}
	}

	return PerceptualHash{
		Algorithm: "spectral-8x8",
		Bits:      64,
		Value:     fmt.Sprintf("%016x", bits),
	}, nil
}

func perceptualBands(samples []float64, bands int) []float64 {
	if len(samples) == 0 {
		return make([]float64, bands)
	}

	if len(samples) > 8192 {
		samples = samples[:8192]
	}

	windowSize := nextPowerOfTwo(len(samples))
	if windowSize < 256 {
		windowSize = 256
	}

	frame := make([]float64, windowSize)
	copy(frame, samples)
	applyHannWindow(frame)

	fft := fourier.NewFFT(windowSize)
	mags := magnitudeSpectrum(fft.Coefficients(nil, frame))
	energies := make([]float64, bands)
	if len(mags) <= 1 {
		return energies
	}

	bandWidth := max(1, (len(mags)-1)/bands)
	for i := 1; i < len(mags); i++ {
		band := min(bands-1, (i-1)/bandWidth)
		energies[band] += mags[i] * mags[i]
	}

	var total float64
	for _, value := range energies {
		total += value
	}
	if total > 0 {
		for i := range energies {
			energies[i] /= total
		}
	}

	return energies
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
