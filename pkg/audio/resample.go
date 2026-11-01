package audio

import (
	"errors"
	"fmt"
	"math"
)

// Resample resamples the given PCMBuffer to targetRate using band-limited
// windowed-sinc (polyphase FIR) interpolation with anti-aliasing low-pass filtering.
func Resample(buffer PCMBuffer, targetRate int) (PCMBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return PCMBuffer{}, fmt.Errorf("resample: %w", err)
	}
	if targetRate <= 0 {
		return PCMBuffer{}, errors.New("resample: target sample rate must be greater than zero")
	}
	if targetRate == buffer.SampleRate {
		clone := buffer.Clone()
		clone.SampleRate = targetRate
		return clone, nil
	}

	inRate := float64(buffer.SampleRate)
	outRate := float64(targetRate)
	ratio := outRate / inRate

	inFrames := buffer.Frames()
	outFrames := int(math.Round(float64(inFrames) * ratio))
	if outFrames < 1 {
		outFrames = 1
	}

	channels := buffer.Channels

	// Anti-aliasing cutoff:
	// When downsampling (ratio < 1.0), cutoff must be placed at the output Nyquist
	// frequency (scaled by 0.95 for a clean transition band before Nyquist).
	// When upsampling (ratio >= 1.0), cutoff is at input Nyquist (0.95).
	cutoff := 0.95
	if ratio < 1.0 {
		cutoff = ratio * 0.95
	}

	// Filter half-width in input samples.
	// Base filter length is 16 zero-crossings.
	const baseRadius = 16
	filterRadius := int(math.Ceil(float64(baseRadius) / cutoff))
	if filterRadius < 8 {
		filterRadius = 8
	} else if filterRadius > 128 {
		filterRadius = 128
	}

	// Precompute 512-phase polyphase filter table.
	// Each phase p corresponds to fractional delay delta = p / 512.
	const numPhases = 512
	filterTaps := 2 * filterRadius
	polyTable := make([][]float64, numPhases)

	for p := 0; p < numPhases; p++ {
		delta := float64(p) / float64(numPhases)
		weights := make([]float64, filterTaps)
		var sum float64

		for i := 0; i < filterTaps; i++ {
			// Offset relative to fractional source position
			k := i - filterRadius + 1
			t := float64(k) - delta

			// Band-limited sinc
			x := cutoff * t
			sinc := 1.0
			if math.Abs(x) > 1e-9 {
				sinc = math.Sin(math.Pi*x) / (math.Pi * x)
			}

			// Blackman window over [-filterRadius, filterRadius]
			absT := math.Abs(t)
			var w float64
			if absT <= float64(filterRadius) {
				w = 0.42 + 0.5*math.Cos(math.Pi*t/float64(filterRadius)) + 0.08*math.Cos(2*math.Pi*t/float64(filterRadius))
			}

			weight := cutoff * sinc * w
			weights[i] = weight
			sum += weight
		}

		// Normalize unity DC gain
		if math.Abs(sum) > 1e-12 {
			for i := range weights {
				weights[i] /= sum
			}
		}
		polyTable[p] = weights
	}

	outData := make([]float64, outFrames*channels)

	for m := 0; m < outFrames; m++ {
		// Exact continuous source position
		srcPos := float64(m) / ratio
		srcCenter := int(math.Floor(srcPos))
		frac := srcPos - float64(srcCenter)

		phase := int(math.Round(frac * float64(numPhases)))
		if phase >= numPhases {
			phase = numPhases - 1
		}

		weights := polyTable[phase]

		for c := 0; c < channels; c++ {
			var acc float64
			for i := 0; i < filterTaps; i++ {
				idx := srcCenter - filterRadius + 1 + i
				if idx >= 0 && idx < inFrames {
					acc += buffer.Data[idx*channels+c] * weights[i]
				}
			}
			outData[m*channels+c] = acc
		}
	}

	return PCMBuffer{
		SampleRate: targetRate,
		Channels:   channels,
		BitDepth:   buffer.BitDepth,
		Data:       outData,
		Metadata:   buffer.Clone().Metadata,
	}, nil
}
