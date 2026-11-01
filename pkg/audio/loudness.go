package audio

import (
	"math"
)

// MeasureLoudness calculates the ITU-R BS.1770-4 integrated loudness (in LUFS)
// and 4x oversampled true peak (in dBTP) across all channels of the buffer.
func MeasureLoudness(buffer PCMBuffer) (integratedLUFS float64, truePeakDBTP float64) {
	if len(buffer.Data) == 0 || buffer.Channels <= 0 || buffer.SampleRate <= 0 {
		return -120.0, -120.0
	}

	numFrames := buffer.Frames()
	numChannels := buffer.Channels
	sampleRate := float64(buffer.SampleRate)

	// Deinterleave channels
	channels := make([][]float64, numChannels)
	for c := 0; c < numChannels; c++ {
		channels[c] = make([]float64, numFrames)
		for i := 0; i < numFrames; i++ {
			channels[c][i] = buffer.Data[i*numChannels+c]
		}
	}

	// 1. Measure 4x oversampled True Peak (dBTP) on raw channel data
	_, truePeakDBTP = measureTruePeakChannels(channels)

	// 2. Measure ITU-R BS.1770-4 Integrated Loudness (LUFS)
	integratedLUFS = measureLUFSChannels(channels, sampleRate)

	return integratedLUFS, truePeakDBTP
}

// MeasureLUFS returns the ITU-R BS.1770-4 gated integrated loudness in LUFS.
func MeasureLUFS(buffer PCMBuffer) float64 {
	lufs, _ := MeasureLoudness(buffer)
	return lufs
}

// MeasureTruePeak returns the maximum 4x oversampled true peak across all channels.
func MeasureTruePeak(buffer PCMBuffer) (linearPeak float64, truePeakDBTP float64) {
	if len(buffer.Data) == 0 || buffer.Channels <= 0 || buffer.SampleRate <= 0 {
		return 0, -120.0
	}

	numFrames := buffer.Frames()
	numChannels := buffer.Channels
	channels := make([][]float64, numChannels)
	for c := 0; c < numChannels; c++ {
		channels[c] = make([]float64, numFrames)
		for i := 0; i < numFrames; i++ {
			channels[c][i] = buffer.Data[i*numChannels+c]
		}
	}

	return measureTruePeakChannels(channels)
}

// kWeightingCoefficients returns the biquad filter coefficients for Stage 1 (pre-filter)
// and Stage 2 (RLB high-pass) of ITU-R BS.1770-4 K-weighting at the given sample rate.
func kWeightingCoefficients(sampleRate float64) (b1, a1, b2, a2 [3]float64) {
	// Stage 1: High shelf filter (+4 dB at 1681.97 Hz)
	f0_1 := 1681.974450955533
	gainDB := 3.999843853973347
	q1 := 0.7071752369554196

	k1 := math.Tan(math.Pi * f0_1 / sampleRate)
	vh := math.Pow(10.0, gainDB/20.0)
	vb := math.Pow(vh, 0.5)

	a0_1 := 1.0 + (k1 / q1) + (k1 * k1)
	b1[0] = (vh + (vb*k1/q1) + (k1 * k1)) / a0_1
	b1[1] = (2.0 * ((k1 * k1) - vh)) / a0_1
	b1[2] = (vh - (vb*k1/q1) + (k1 * k1)) / a0_1
	a1[0] = 1.0
	a1[1] = (2.0 * ((k1 * k1) - 1.0)) / a0_1
	a1[2] = (1.0 - (k1 / q1) + (k1 * k1)) / a0_1

	// Stage 2: High pass filter (RLB, 38.14 Hz)
	f0_2 := 38.13547087602444
	q2 := 0.5003270373238773

	k2 := math.Tan(math.Pi * f0_2 / sampleRate)
	a0_2 := 1.0 + (k2 / q2) + (k2 * k2)
	b2[0] = 1.0 / a0_2
	b2[1] = -2.0 / a0_2
	b2[2] = 1.0 / a0_2
	a2[0] = 1.0
	a2[1] = (2.0 * ((k2 * k2) - 1.0)) / a0_2
	a2[2] = (1.0 - (k2 / q2) + (k2 * k2)) / a0_2

	return b1, a1, b2, a2
}

func applyBiquad(in []float64, b, a [3]float64) []float64 {
	out := make([]float64, len(in))
	b0, b1, b2 := b[0], b[1], b[2]
	a1, a2 := a[1], a[2]

	var x1, x2, y1, y2 float64
	for i, x0 := range in {
		y0 := b0*x0 + b1*x1 + b2*x2 - a1*y1 - a2*y2
		out[i] = y0
		x2 = x1
		x1 = x0
		y2 = y1
		y1 = y0
	}
	return out
}

func channelWeight(channelIdx, totalChannels int) float64 {
	// Standard BS.1770-4 channel weights:
	// L, R, C: 1.0 (0 dB)
	// Ls, Rs (surround): 1.41 (+1.5 dB)
	// LFE (if 5.1/6-ch at index 3): 0.0
	if totalChannels == 6 && channelIdx == 3 {
		return 0.0 // LFE channel excluded from loudness
	}
	if totalChannels >= 4 && (channelIdx == totalChannels-2 || channelIdx == totalChannels-1) {
		return 1.41 // Surround channels
	}
	return 1.0
}

func measureLUFSChannels(channels [][]float64, sampleRate float64) float64 {
	numChannels := len(channels)
	if numChannels == 0 {
		return -120.0
	}
	numFrames := len(channels[0])
	if numFrames == 0 {
		return -120.0
	}

	// 1. Apply K-weighting filter to each channel
	b1, a1, b2, a2 := kWeightingCoefficients(sampleRate)
	kWeighted := make([][]float64, numChannels)
	weights := make([]float64, numChannels)

	for c := 0; c < numChannels; c++ {
		stage1 := applyBiquad(channels[c], b1, a1)
		kWeighted[c] = applyBiquad(stage1, b2, a2)
		weights[c] = channelWeight(c, numChannels)
	}

	// Gating parameters: 400 ms window, 100 ms step
	windowFrames := int(math.Round(0.400 * sampleRate))
	stepFrames := int(math.Round(0.100 * sampleRate))
	if stepFrames < 1 {
		stepFrames = 1
	}

	// Handle short audio (< 400 ms): compute un-gated K-weighted loudness
	if numFrames < windowFrames {
		var totalPower float64
		for c := 0; c < numChannels; c++ {
			if weights[c] == 0 {
				continue
			}
			var sumSq float64
			for _, s := range kWeighted[c] {
				sumSq += s * s
			}
			totalPower += weights[c] * (sumSq / float64(numFrames))
		}
		if totalPower <= 1e-12 {
			return -120.0
		}
		return math.Max(-120.0, -0.691+10.0*math.Log10(totalPower))
	}

	// 2. Compute mean square powers for 400ms gating blocks
	type block struct {
		power float64
		lufs  float64
	}
	var blocks []block

	for start := 0; start+windowFrames <= numFrames; start += stepFrames {
		var blockPower float64
		for c := 0; c < numChannels; c++ {
			if weights[c] == 0 {
				continue
			}
			var sumSq float64
			for i := start; i < start+windowFrames; i++ {
				s := kWeighted[c][i]
				sumSq += s * s
			}
			blockPower += weights[c] * (sumSq / float64(windowFrames))
		}

		l := -120.0
		if blockPower > 1e-12 {
			l = -0.691 + 10.0*math.Log10(blockPower)
		}
		blocks = append(blocks, block{power: blockPower, lufs: l})
	}

	if len(blocks) == 0 {
		return -120.0
	}

	// 3. Absolute threshold gating (Gamma_a = -70.0 LUFS)
	const gammaA = -70.0
	var absPowerSum float64
	var absCount int

	for _, b := range blocks {
		if b.lufs > gammaA {
			absPowerSum += b.power
			absCount++
		}
	}

	if absCount == 0 {
		return -120.0
	}

	zAbs := absPowerSum / float64(absCount)
	lAbs := -0.691 + 10.0*math.Log10(zAbs)

	// 4. Relative threshold gating (Gamma_r = L_abs - 10.0 LUFS)
	gammaR := lAbs - 10.0
	var finalPowerSum float64
	var finalCount int

	for _, b := range blocks {
		if b.lufs > gammaA && b.lufs > gammaR {
			finalPowerSum += b.power
			finalCount++
		}
	}

	if finalCount == 0 {
		return math.Max(-120.0, lAbs)
	}

	zFinal := finalPowerSum / float64(finalCount)
	if zFinal <= 1e-12 {
		return -120.0
	}

	return math.Max(-120.0, -0.691+10.0*math.Log10(zFinal))
}

func measureTruePeakChannels(channels [][]float64) (linearPeak float64, dbtp float64) {
	numChannels := len(channels)
	if numChannels == 0 {
		return 0, -120.0
	}
	numFrames := len(channels[0])
	if numFrames == 0 {
		return 0, -120.0
	}

	// Precompute 4-phase polyphase interpolation filter (12 taps per phase, sinc * Hann)
	var polyFilter [4][12]float64
	for p := 0; p < 4; p++ {
		delta := float64(p) / 4.0
		var sum float64
		for k := 0; k < 12; k++ {
			t := float64(k-5) - delta
			sinc := 1.0
			if math.Abs(t) > 1e-9 {
				sinc = math.Sin(math.Pi*t) / (math.Pi * t)
			}
			w := 0.5 + 0.5*math.Cos(math.Pi*t/6.0)
			if math.Abs(t) > 6.0 {
				w = 0
			}
			polyFilter[p][k] = sinc * w
			sum += polyFilter[p][k]
		}
		if sum > 0 {
			for k := 0; k < 12; k++ {
				polyFilter[p][k] /= sum
			}
		}
	}

	var maxPeak float64
	for c := 0; c < numChannels; c++ {
		data := channels[c]
		for n := 0; n < numFrames; n++ {
			for p := 0; p < 4; p++ {
				var val float64
				for k := 0; k < 12; k++ {
					idx := n + k - 5
					if idx >= 0 && idx < numFrames {
						val += data[idx] * polyFilter[p][k]
					}
				}
				abs := math.Abs(val)
				if abs > maxPeak {
					maxPeak = abs
				}
			}
		}
	}

	if maxPeak <= 1e-6 {
		return maxPeak, -120.0
	}

	return maxPeak, 20.0 * math.Log10(maxPeak)
}
