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

const spectralAnalyzerVersion = "0.2.0"

type SpectralAnalyzer struct {
	registry *zaudio.Registry
}

func NewSpectralAnalyzer(registry *zaudio.Registry) (*SpectralAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &SpectralAnalyzer{registry: registry}, nil
}

func (a *SpectralAnalyzer) Name() string {
	return "spectral"
}

func (a *SpectralAnalyzer) Version() string {
	return spectralAnalyzerVersion
}

func (a *SpectralAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("spectral analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze spectral features for %q: %w", sample.Path, err)
	}

	stats := computeSpectral(decoded.Buffer)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"spectral_centroid_hz":  stats.CentroidHz,
			"spectral_rolloff_hz":   stats.RolloffHz,
			"spectral_flux":         stats.Flux,
			"zero_crossing_rate":    stats.ZeroCrossingRate,
			"dominant_frequency_hz": stats.DominantFrequencyHz,
			"spectral_flatness":     stats.Flatness,
			"spectral_bandwidth_hz": stats.BandwidthHz,
			"spectral_contrast_0":   stats.Contrast[0],
			"spectral_contrast_1":   stats.Contrast[1],
			"spectral_contrast_2":   stats.Contrast[2],
			"spectral_contrast_3":   stats.Contrast[3],
			"spectral_contrast_4":   stats.Contrast[4],
			"spectral_contrast_5":   stats.Contrast[5],
			"spectral_contrast_6":   stats.Contrast[6],
		},
		Attributes: map[string]string{
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

type SpectralStats struct {
	CentroidHz          float64
	RolloffHz           float64
	Flux                float64
	ZeroCrossingRate    float64
	DominantFrequencyHz float64
	Flatness            float64
	BandwidthHz         float64
	Contrast            [7]float64
}

// computeSpectral follows standard MIR feature definitions for spectral
// bandwidth and uses the sub-band contrast approach described by Jiang et al.
// (2002), "Music type classification by spectral contrast feature."
func computeSpectral(buffer zaudio.PCMBuffer) SpectralStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return SpectralStats{}
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
	average := make([]float64, windowSize/2+1)
	frame := make([]float64, windowSize)
	mags := make([]float64, windowSize/2+1)
	prev := make([]float64, windowSize/2+1)
	var (
		frameCount   int
		totalFlux    float64
		havePrev     bool
		contrastSums [7]float64
	)

	for start := 0; start < len(mono); start += hopSize {
		clear(frame)
		end := start + windowSize
		if end > len(mono) {
			end = len(mono)
		}
		copy(frame, mono[start:end])
		applyHannWindow(frame)

		coeff := fft.Coefficients(nil, frame)
		current := magnitudeSpectrumInto(coeff, mags)

		for i, mag := range current {
			average[i] += mag
		}

		if havePrev {
			var flux float64
			for i, mag := range current {
				diff := mag - prev[i]
				if diff > 0 {
					flux += diff
				}
			}
			totalFlux += flux / float64(len(current))
		}

		contrast := spectralContrast(current, buffer.SampleRate, fft, len(contrastSums))
		for i := range contrastSums {
			contrastSums[i] += contrast[i]
		}

		copy(prev, current)
		havePrev = true
		frameCount++
		if end == len(mono) {
			break
		}
	}

	if frameCount == 0 {
		return SpectralStats{}
	}

	for i := range average {
		average[i] /= float64(frameCount)
	}

	centroid := spectralCentroid(average, buffer.SampleRate, fft)
	var contrast [7]float64
	for i := range contrast {
		contrast[i] = contrastSums[i] / float64(frameCount)
	}

	return SpectralStats{
		CentroidHz:          centroid,
		RolloffHz:           spectralRolloff(average, buffer.SampleRate, fft, 0.85),
		Flux:                totalFlux / math.Max(1, float64(frameCount-1)),
		ZeroCrossingRate:    zeroCrossingRate(mono),
		DominantFrequencyHz: dominantFrequency(average, buffer.SampleRate, fft),
		Flatness:            spectralFlatness(average),
		BandwidthHz:         spectralBandwidth(average, buffer.SampleRate, fft, centroid),
		Contrast:            contrast,
	}
}

func mixDownMono(buffer zaudio.PCMBuffer) []float64 {
	if buffer.Channels <= 1 {
		return append([]float64(nil), buffer.Data...)
	}

	frames := buffer.Frames()
	mono := make([]float64, frames)
	for frame := 0; frame < frames; frame++ {
		var sum float64
		for ch := 0; ch < buffer.Channels; ch++ {
			sum += buffer.Data[frame*buffer.Channels+ch]
		}
		mono[frame] = sum / float64(buffer.Channels)
	}
	return mono
}

func applyHannWindow(frame []float64) {
	n := len(frame)
	if n <= 1 {
		return
	}
	denom := float64(n - 1)
	for i := range frame {
		frame[i] *= 0.5 * (1 - math.Cos((2*math.Pi*float64(i))/denom))
	}
}

func magnitudeSpectrum(coeff []complex128) []float64 {
	limit := len(coeff)/2 + 1
	mags := make([]float64, limit)
	return magnitudeSpectrumInto(coeff, mags)
}

func magnitudeSpectrumInto(coeff []complex128, mags []float64) []float64 {
	limit := len(coeff)/2 + 1
	if len(mags) < limit {
		limit = len(mags)
	}
	for i := 0; i < limit; i++ {
		mags[i] = cmplxAbs(coeff[i])
	}
	return mags[:limit]
}

func spectralCentroid(mags []float64, sampleRate int, fft *fourier.FFT) float64 {
	var weighted, total float64
	for i, mag := range mags {
		freq := fft.Freq(i) * float64(sampleRate)
		weighted += freq * mag
		total += mag
	}
	if total == 0 {
		return 0
	}
	return weighted / total
}

func spectralRolloff(mags []float64, sampleRate int, fft *fourier.FFT, threshold float64) float64 {
	var total float64
	for _, mag := range mags {
		total += mag
	}
	if total == 0 {
		return 0
	}

	target := total * threshold
	var cumulative float64
	for i, mag := range mags {
		cumulative += mag
		if cumulative >= target {
			return fft.Freq(i) * float64(sampleRate)
		}
	}
	return 0
}

func spectralBandwidth(mags []float64, sampleRate int, fft *fourier.FFT, centroid float64) float64 {
	var (
		weighted float64
		total    float64
	)
	for i, mag := range mags {
		freq := fft.Freq(i) * float64(sampleRate)
		diff := freq - centroid
		weighted += mag * diff * diff
		total += mag
	}
	if total == 0 {
		return 0
	}
	return math.Sqrt(weighted / total)
}

func dominantFrequency(mags []float64, sampleRate int, fft *fourier.FFT) float64 {
	var (
		index int
		max   float64
	)
	for i := 1; i < len(mags); i++ {
		if mags[i] > max {
			max = mags[i]
			index = i
		}
	}
	return fft.Freq(index) * float64(sampleRate)
}

func spectralFlatness(mags []float64) float64 {
	var (
		sumLog float64
		sum    float64
		count  int
	)
	for _, mag := range mags {
		if mag <= 0 {
			continue
		}
		sumLog += math.Log(mag)
		sum += mag
		count++
	}
	if count == 0 || sum == 0 {
		return 0
	}
	geo := math.Exp(sumLog / float64(count))
	arith := sum / float64(count)
	return geo / arith
}

func spectralContrast(mags []float64, sampleRate int, fft *fourier.FFT, bandCount int) []float64 {
	contrast := make([]float64, bandCount)
	if len(mags) == 0 || bandCount <= 0 {
		return contrast
	}

	nyquist := float64(sampleRate) / 2
	lower := 0.0
	upper := 200.0
	epsilon := 1e-12

	for band := 0; band < bandCount; band++ {
		if band == bandCount-1 || upper >= nyquist {
			upper = nyquist
		}

		values := make([]float64, 0, len(mags))
		for i, mag := range mags {
			freq := fft.Freq(i) * float64(sampleRate)
			if freq < lower {
				continue
			}
			if band == bandCount-1 {
				if freq > upper {
					continue
				}
			} else if freq >= upper {
				continue
			}
			values = append(values, mag)
		}

		if len(values) > 0 {
			sort.Float64s(values)
			low := percentile(values, 0.1)
			high := percentile(values, 0.9)
			contrast[band] = 20*math.Log10(high+epsilon) - 20*math.Log10(low+epsilon)
		}

		lower = upper
		upper *= 2
	}

	return contrast
}

func zeroCrossingRate(samples []float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	crossings := 0
	prev := samples[0]
	for _, current := range samples[1:] {
		if (prev >= 0 && current < 0) || (prev < 0 && current >= 0) {
			crossings++
		}
		prev = current
	}
	return float64(crossings) / float64(len(samples)-1)
}

func nextPowerOfTwo(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func cmplxAbs(v complex128) float64 {
	return math.Hypot(real(v), imag(v))
}
