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

const mfccAnalyzerVersion = "0.1.0"

type MFCCAnalyzer struct {
	registry *zaudio.Registry
}

func NewMFCCAnalyzer(registry *zaudio.Registry) (*MFCCAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &MFCCAnalyzer{registry: registry}, nil
}

func (a *MFCCAnalyzer) Name() string {
	return "mfcc"
}

func (a *MFCCAnalyzer) Version() string {
	return mfccAnalyzerVersion
}

func (a *MFCCAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("mfcc analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze mfcc for %q: %w", sample.Path, err)
	}

	coeffs := computeMFCC(decoded.Buffer, 13, 26)
	metrics := make(map[string]float64, len(coeffs))
	for i, value := range coeffs {
		metrics[fmt.Sprintf("mfcc_%d", i)] = value
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics:     metrics,
		FeatureVectors: []core.FeatureVector{{
			SampleID:   sample.ID,
			Namespace:  "mfcc",
			Version:    a.Version(),
			Values:     coeffs,
			Dimensions: len(coeffs),
			CreatedAt:  time.Now().UTC(),
		}},
	}, nil
}

func computeMFCC(buffer zaudio.PCMBuffer, coeffCount, filterCount int) []float64 {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 || coeffCount <= 0 || filterCount <= 0 {
		return nil
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
	filterBank := melFilterBank(filterCount, windowSize, buffer.SampleRate)
	accum := make([]float64, coeffCount)
	frame := make([]float64, windowSize)
	power := make([]float64, windowSize/2+1)
	energies := make([]float64, len(filterBank))
	logEnergies := make([]float64, len(filterBank))
	frameCoeffs := make([]float64, coeffCount)
	frameCount := 0

	for start := 0; start < len(mono); start += hopSize {
		clear(frame)
		end := start + windowSize
		if end > len(mono) {
			end = len(mono)
		}
		copy(frame, mono[start:end])
		applyHannWindow(frame)

		coeff := fft.Coefficients(nil, frame)
		current := powerSpectrumInto(coeff, power)
		applyFilterBankInto(current, filterBank, energies)
		for i, energy := range energies {
			if energy <= 1e-12 {
				energy = 1e-12
			}
			logEnergies[i] = math.Log(energy)
		}

		dctInto(logEnergies, frameCoeffs)
		for i, value := range frameCoeffs {
			accum[i] += value
		}
		frameCount++

		if end == len(mono) {
			break
		}
	}

	if frameCount == 0 {
		return nil
	}

	for i := range accum {
		accum[i] /= float64(frameCount)
	}

	return accum
}

func powerSpectrum(coeff []complex128) []float64 {
	limit := len(coeff)/2 + 1
	power := make([]float64, limit)
	return powerSpectrumInto(coeff, power)
}

func powerSpectrumInto(coeff []complex128, power []float64) []float64 {
	limit := len(coeff)/2 + 1
	if len(power) < limit {
		limit = len(power)
	}
	for i := 0; i < limit; i++ {
		realPart := real(coeff[i])
		imagPart := imag(coeff[i])
		power[i] = realPart*realPart + imagPart*imagPart
	}
	return power[:limit]
}

func melFilterBank(filterCount, fftSize, sampleRate int) [][]float64 {
	binCount := fftSize/2 + 1
	lowMel := hzToMel(0)
	highMel := hzToMel(float64(sampleRate) / 2)

	melPoints := make([]float64, filterCount+2)
	for i := range melPoints {
		melPoints[i] = lowMel + (highMel-lowMel)*float64(i)/float64(len(melPoints)-1)
	}

	bins := make([]int, len(melPoints))
	for i, mel := range melPoints {
		hz := melToHz(mel)
		bins[i] = int(math.Floor((float64(fftSize) + 1) * hz / float64(sampleRate)))
		if bins[i] >= binCount {
			bins[i] = binCount - 1
		}
	}

	filters := make([][]float64, filterCount)
	for m := 1; m <= filterCount; m++ {
		filter := make([]float64, binCount)
		left, center, right := bins[m-1], bins[m], bins[m+1]
		if center == left {
			center++
		}
		if right == center {
			right++
		}
		for k := left; k < center && k < binCount; k++ {
			filter[k] = float64(k-left) / float64(center-left)
		}
		for k := center; k < right && k < binCount; k++ {
			filter[k] = float64(right-k) / float64(right-center)
		}
		filters[m-1] = filter
	}

	return filters
}

func applyFilterBank(power []float64, filters [][]float64) []float64 {
	energies := make([]float64, len(filters))
	applyFilterBankInto(power, filters, energies)
	return energies
}

func applyFilterBankInto(power []float64, filters [][]float64, energies []float64) {
	for i, filter := range filters {
		var sum float64
		for bin, weight := range filter {
			if bin >= len(power) {
				break
			}
			sum += power[bin] * weight
		}
		energies[i] = sum
	}
}

func dct(values []float64, coeffCount int) []float64 {
	if coeffCount > len(values) {
		coeffCount = len(values)
	}
	coeffs := make([]float64, coeffCount)
	dctInto(values, coeffs)
	return coeffs
}

func dctInto(values, coeffs []float64) {
	if len(coeffs) > len(values) {
		coeffs = coeffs[:len(values)]
	}
	n := float64(len(values))
	for k := range coeffs {
		var sum float64
		for i, value := range values {
			sum += value * math.Cos(math.Pi*float64(k)*(float64(i)+0.5)/n)
		}
		coeffs[k] = sum
	}
}

func hzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func melToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}
