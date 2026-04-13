package clap

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/dsp/fourier"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

// Default CLAP (laion/clap-htsat-unfused) feature extraction parameters.
const (
	DefaultSampleRate    = 48000
	DefaultNumMelBins    = 64
	DefaultFFTWindowSize = 1024
	DefaultHopLength     = 320
	DefaultFreqMin       = 50.0
	DefaultFreqMax       = 14000.0
	DefaultMaxSamples    = 480000 // 10 s at 48 kHz
)

// FeatureExtractorConfig holds the parameters for the log-mel spectrogram
// computation, matching the HuggingFace ClapFeatureExtractor defaults for
// laion/clap-htsat-unfused.
type FeatureExtractorConfig struct {
	SampleRate    int
	NumMelBins    int
	FFTWindowSize int
	HopLength     int
	FreqMin       float64
	FreqMax       float64
	MaxSamples    int
}

// DefaultFeatureExtractorConfig returns the default CLAP feature extractor
// configuration.
func DefaultFeatureExtractorConfig() FeatureExtractorConfig {
	return FeatureExtractorConfig{
		SampleRate:    DefaultSampleRate,
		NumMelBins:    DefaultNumMelBins,
		FFTWindowSize: DefaultFFTWindowSize,
		HopLength:     DefaultHopLength,
		FreqMin:       DefaultFreqMin,
		FreqMax:       DefaultFreqMax,
		MaxSamples:    DefaultMaxSamples,
	}
}

// NumFrames returns the number of mel spectrogram time frames that this
// configuration produces (using centre-padding STFT, matching librosa default).
func (c FeatureExtractorConfig) NumFrames() int {
	// With centre=True: 1 + MaxSamples / HopLength
	return 1 + c.MaxSamples/c.HopLength
}

// FeatureExtractor converts a raw PCM buffer to a log-mel spectrogram suitable
// for input to the CLAP audio encoder.
type FeatureExtractor struct {
	cfg     FeatureExtractorConfig
	fft     *fourier.FFT
	filters [][]float64
	hannWin []float64
}

// NewFeatureExtractor creates a FeatureExtractor.
func NewFeatureExtractor(cfg FeatureExtractorConfig) (*FeatureExtractor, error) {
	if cfg.SampleRate <= 0 || cfg.NumMelBins <= 0 || cfg.FFTWindowSize < 2 ||
		cfg.HopLength <= 0 || cfg.MaxSamples <= 0 {
		return nil, fmt.Errorf("invalid FeatureExtractorConfig")
	}

	fftN := cfg.FFTWindowSize
	fft := fourier.NewFFT(fftN)
	filters := melFilterBankHTK(cfg.NumMelBins, fftN, cfg.SampleRate, cfg.FreqMin, cfg.FreqMax)

	hannWin := make([]float64, fftN)
	for i := range hannWin {
		hannWin[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftN-1)))
	}

	return &FeatureExtractor{cfg: cfg, fft: fft, filters: filters, hannWin: hannWin}, nil
}

// Extract converts mono float64 PCM samples at srcRate Hz into a float32
// log-mel spectrogram.  The returned slice has length
// cfg.NumMelBins * cfg.NumFrames() and is laid out as
// [mel_bin][time_frame] (row-major), ready to be reshaped into the ONNX
// audio encoder input [1, 1, NumMelBins, NumFrames].
func (fe *FeatureExtractor) Extract(samples []float64, srcRate int) ([]float32, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("empty audio")
	}
	if srcRate <= 0 {
		return nil, fmt.Errorf("invalid source sample rate %d", srcRate)
	}

	// 1. Resample to target sample rate.
	mono := resampleLinear(samples, srcRate, fe.cfg.SampleRate)

	// 2. Pad / truncate to exactly MaxSamples.
	if len(mono) > fe.cfg.MaxSamples {
		mono = mono[:fe.cfg.MaxSamples]
	} else if len(mono) < fe.cfg.MaxSamples {
		padded := make([]float64, fe.cfg.MaxSamples)
		copy(padded, mono)
		mono = padded
	}

	// 3. Compute power log-mel spectrogram in [mel][frame] layout.
	return fe.computeLogMelSpectrogram(mono), nil
}

// ExtractFromBuffer extracts a log-mel spectrogram from a PCMBuffer, mixing
// down to mono first if necessary.
func (fe *FeatureExtractor) ExtractFromBuffer(buf zaudio.PCMBuffer) ([]float32, error) {
	mono := mixDownToMono(buf)
	return fe.Extract(mono, buf.SampleRate)
}

// computeLogMelSpectrogram does the heavy lifting: STFT with Hann window,
// power spectrum, mel filterbank, dB scaling, and min-max normalisation.
// Output layout: [NumMelBins * NumFrames] in row-major [mel][frame] order.
func (fe *FeatureExtractor) computeLogMelSpectrogram(samples []float64) []float32 {
	cfg := fe.cfg
	halfWin := cfg.FFTWindowSize / 2
	numFrames := fe.cfg.NumFrames()

	// Centre-pad the signal by n_fft/2 zeros on each side (matching
	// librosa centre=True behaviour, which the HuggingFace feature extractor
	// replicates).
	padded := make([]float64, len(samples)+2*halfWin)
	copy(padded[halfWin:], samples)

	logMel := make([]float64, cfg.NumMelBins*numFrames)
	frame := make([]float64, cfg.FFTWindowSize)
	binCount := cfg.FFTWindowSize/2 + 1

	for f := 0; f < numFrames; f++ {
		start := f * cfg.HopLength

		// Fill frame, applying Hann window.
		for i := 0; i < cfg.FFTWindowSize; i++ {
			idx := start + i
			if idx < len(padded) {
				frame[i] = padded[idx] * fe.hannWin[i]
			} else {
				frame[i] = 0
			}
		}

		// FFT and power spectrum.
		coeff := fe.fft.Coefficients(nil, frame)

		// Mel filterbank + log scaling.
		for m, filter := range fe.filters {
			var energy float64
			for bin, weight := range filter {
				if bin >= binCount {
					break
				}
				r, im := real(coeff[bin]), imag(coeff[bin])
				energy += (r*r + im*im) * weight
			}
			// dB scale: 10 * log10(max(energy, 1e-10))
			logMel[m*numFrames+f] = 10 * math.Log10(math.Max(energy, 1e-10))
		}
	}

	// Min-max normalise to [0, 1].
	minV, maxV := logMel[0], logMel[0]
	for _, v := range logMel {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	out := make([]float32, len(logMel))
	rangeV := maxV - minV
	if rangeV < 1e-12 {
		return out
	}
	invRange := float32(1.0 / rangeV)
	for i, v := range logMel {
		out[i] = float32(v-minV) * invRange
	}
	return out
}

// resampleLinear resamples data from fromRate to toRate using linear
// interpolation.
func resampleLinear(data []float64, fromRate, toRate int) []float64 {
	if fromRate == toRate {
		return append([]float64(nil), data...)
	}
	ratio := float64(fromRate) / float64(toRate)
	newLen := int(math.Round(float64(len(data)) / ratio))
	if newLen <= 0 {
		return nil
	}
	out := make([]float64, newLen)
	last := float64(len(data) - 1)
	for i := range out {
		pos := float64(i) * ratio
		if pos >= last {
			out[i] = data[len(data)-1]
			continue
		}
		lo := int(pos)
		out[i] = data[lo] + (data[lo+1]-data[lo])*(pos-float64(lo))
	}
	return out
}

// melFilterBankHTK builds a triangular mel filterbank using the HTK mel scale
// (hzToMel = 2595*log10(1+hz/700)) with explicit frequency bounds.
// Returns filterCount filters, each of length fftSize/2+1.
func melFilterBankHTK(filterCount, fftSize, sampleRate int, fmin, fmax float64) [][]float64 {
	binCount := fftSize/2 + 1
	lowMel := clapHzToMel(fmin)
	highMel := clapHzToMel(fmax)

	melPoints := make([]float64, filterCount+2)
	for i := range melPoints {
		melPoints[i] = lowMel + (highMel-lowMel)*float64(i)/float64(filterCount+1)
	}

	bins := make([]int, len(melPoints))
	for i, mel := range melPoints {
		hz := clapMelToHz(mel)
		b := int(math.Floor(float64(fftSize+1) * hz / float64(sampleRate)))
		if b < 0 {
			b = 0
		}
		if b >= binCount {
			b = binCount - 1
		}
		bins[i] = b
	}

	filters := make([][]float64, filterCount)
	for m := 1; m <= filterCount; m++ {
		filter := make([]float64, binCount)
		left, center, right := bins[m-1], bins[m], bins[m+1]
		if center <= left {
			center = left + 1
		}
		if right <= center {
			right = center + 1
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

func clapHzToMel(hz float64) float64 {
	return 2595.0 * math.Log10(1.0+hz/700.0)
}

func clapMelToHz(mel float64) float64 {
	return 700.0 * (math.Pow(10.0, mel/2595.0) - 1.0)
}

// mixDownToMono converts a multi-channel PCMBuffer to a mono float64 slice.
func mixDownToMono(buf zaudio.PCMBuffer) []float64 {
	if buf.Channels <= 1 {
		return append([]float64(nil), buf.Data...)
	}
	frames := buf.Frames()
	mono := make([]float64, frames)
	for frame := 0; frame < frames; frame++ {
		var sum float64
		for ch := 0; ch < buf.Channels; ch++ {
			sum += buf.Data[frame*buf.Channels+ch]
		}
		mono[frame] = sum / float64(buf.Channels)
	}
	return mono
}
