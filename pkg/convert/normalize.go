package convert

import (
	"fmt"
	"math"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

type NormalizeMode string

const (
	NormalizePeak NormalizeMode = "peak"
	NormalizeRMS  NormalizeMode = "rms"
	NormalizeLUFS NormalizeMode = "lufs"
)

type NormalizeOptions struct {
	Mode          NormalizeMode
	TargetDBFS    float64
	AllowClipping bool
}

func Normalize(buffer zaudio.PCMBuffer, opts NormalizeOptions) (zaudio.PCMBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return zaudio.PCMBuffer{}, fmt.Errorf("normalize: %w", err)
	}

	mode := opts.Mode
	if mode == "" {
		mode = NormalizePeak
	}

	stats := measureLevel(buffer)

	var currentDBFS float64
	switch mode {
	case NormalizePeak:
		currentDBFS = stats.peakDBFS
	case NormalizeRMS, NormalizeLUFS:
		currentDBFS = stats.rmsDBFS
	default:
		return zaudio.PCMBuffer{}, fmt.Errorf("normalize: unsupported mode %q", mode)
	}

	if currentDBFS <= -120 {
		return zaudio.PCMBuffer{}, fmt.Errorf("normalize: cannot normalize silent audio")
	}

	gain := math.Pow(10, (opts.TargetDBFS-currentDBFS)/20)
	maxScaledPeak := stats.peak * gain
	if maxScaledPeak > 1.0 && !opts.AllowClipping {
		return zaudio.PCMBuffer{}, fmt.Errorf("normalize: requested gain would clip audio (peak %.4f)", maxScaledPeak)
	}

	output := buffer.Clone()
	for i, sample := range output.Data {
		scaled := sample * gain
		if scaled > 1 {
			scaled = 1
		} else if scaled < -1 {
			scaled = -1
		}
		output.Data[i] = scaled
	}

	return output, nil
}

type levelStats struct {
	peak     float64
	peakDBFS float64
	rmsDBFS  float64
}

func measureLevel(buffer zaudio.PCMBuffer) levelStats {
	var peak float64
	var sumSq float64

	for _, sample := range buffer.Data {
		abs := math.Abs(sample)
		if abs > peak {
			peak = abs
		}
		sumSq += sample * sample
	}

	rms := math.Sqrt(sumSq / float64(len(buffer.Data)))

	return levelStats{
		peak:     peak,
		peakDBFS: linearToDBFS(peak),
		rmsDBFS:  linearToDBFS(rms),
	}
}

func linearToDBFS(value float64) float64 {
	if value <= 0 {
		return -120
	}
	return 20 * math.Log10(value)
}
