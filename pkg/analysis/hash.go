package analysis

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zconvert "github.com/darkliquid/zounds/pkg/convert"
	"github.com/darkliquid/zounds/pkg/core"
)

const perceptualHashAnalyzerVersion = "0.2.0"

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

	return bits.OnesCount64(leftValue ^ rightValue), nil
}

var perceptualBandEdges = []float64{130, 210, 340, 550, 890, 1440, 2330, 4000}

func computePerceptualHash(buffer zaudio.PCMBuffer) (PerceptualHash, error) {
	monoData := mixDownMono(buffer)
	if len(monoData) < 16 || buffer.SampleRate <= 0 {
		return PerceptualHash{
			Algorithm: "spectral-8x8",
			Bits:      64,
			Value:     "0000000000000000",
		}, nil
	}

	mono := zaudio.PCMBuffer{
		SampleRate: buffer.SampleRate,
		Channels:   1,
		BitDepth:   buffer.BitDepth,
		Data:       monoData,
	}

	if mono.SampleRate != 8000 {
		resampled, err := zconvert.Resample(mono, zconvert.ResampleOptions{TargetSampleRate: 8000})
		if err != nil {
			return PerceptualHash{}, fmt.Errorf("compute perceptual hash: %w", err)
		}
		mono = resampled
	}

	var maxAbs float64
	for _, v := range mono.Data {
		if abs := math.Abs(v); abs > maxAbs {
			maxAbs = abs
		}
	}
	if maxAbs < 1e-6 {
		return PerceptualHash{
			Algorithm: "spectral-8x8",
			Bits:      64,
			Value:     "0000000000000000",
		}, nil
	}

	const (
		numWindows = 8
		numBands   = 8
	)

	n := len(mono.Data)
	var (
		energies    [numWindows][numBands]float64
		logEnergies [numWindows][numBands]float64
		totalEnergy float64
		sumLog      float64
	)

	for t := 0; t < numWindows; t++ {
		start := t * n / numWindows
		end := (t + 1) * n / numWindows
		segLen := end - start
		if segLen <= 0 {
			for b := 0; b < numBands; b++ {
				logEnergies[t][b] = math.Log(1e-12)
				sumLog += logEnergies[t][b]
			}
			continue
		}

		seg := mono.Data[start:end]
		fftSize := nextPowerOfTwo(segLen)
		if fftSize < 256 {
			fftSize = 256
		} else if fftSize > 4096 {
			fftSize = 4096
			if segLen > fftSize {
				seg = seg[:fftSize]
				segLen = fftSize
			}
		}

		frame := make([]float64, fftSize)
		copy(frame, seg)
		if segLen > 1 {
			for i := 0; i < segLen; i++ {
				frame[i] *= 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(segLen-1)))
			}
		}

		fft := fourier.NewFFT(fftSize)
		mags := magnitudeSpectrum(fft.Coefficients(nil, frame))

		var bandEnergies [numBands]float64
		for k := 1; k < len(mags); k++ {
			freq := float64(k) * 8000.0 / float64(fftSize)
			band := 0
			for band < numBands-1 && freq >= perceptualBandEdges[band] {
				band++
			}
			bandEnergies[band] += mags[k] * mags[k]
		}

		for b := 0; b < numBands; b++ {
			energies[t][b] = bandEnergies[b]
			totalEnergy += bandEnergies[b]
		}
	}

	if totalEnergy <= 0 {
		return PerceptualHash{
			Algorithm: "spectral-8x8",
			Bits:      64,
			Value:     "0000000000000000",
		}, nil
	}

	for t := 0; t < numWindows; t++ {
		for b := 0; b < numBands; b++ {
			normE := energies[t][b] / totalEnergy
			logE := math.Log(normE + 1e-12)
			logEnergies[t][b] = logE
			sumLog += logE
		}
	}

	meanLog := sumLog / float64(numWindows*numBands)

	var hashBits uint64
	for t := 0; t < numWindows; t++ {
		for b := 0; b < numBands; b++ {
			if logEnergies[t][b] >= meanLog {
				bitIndex := uint(t*numBands + b)
				hashBits |= (uint64(1) << bitIndex)
			}
		}
	}

	return PerceptualHash{
		Algorithm: "spectral-8x8",
		Bits:      64,
		Value:     fmt.Sprintf("%016x", hashBits),
	}, nil
}
