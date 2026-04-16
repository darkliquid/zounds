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

const keyAnalyzerVersion = "0.2.0"

var (
	majorProfile = []float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
	minorProfile = []float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}
	noteClasses  = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
)

type KeyAnalyzer struct {
	registry *zaudio.Registry
}

func NewKeyAnalyzer(registry *zaudio.Registry) (*KeyAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &KeyAnalyzer{registry: registry}, nil
}

func (a *KeyAnalyzer) Name() string {
	return "key"
}

func (a *KeyAnalyzer) Version() string {
	return keyAnalyzerVersion
}

func (a *KeyAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("key analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze key for %q: %w", sample.Path, err)
	}

	stats := computeKey(decoded.Buffer)
	attributes := map[string]string{
		"channel_layout": channelLayout(decoded.Buffer.Channels),
	}
	if stats.Key != "" {
		attributes["key"] = stats.Key
		attributes["tonic"] = stats.Tonic
		attributes["mode"] = stats.Mode
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics:     buildKeyMetrics(stats),
		Attributes:  attributes,
	}, nil
}

type KeyStats struct {
	Key        string
	Tonic      string
	Mode       string
	RootIndex  int
	Confidence float64
	Chroma     [12]float64
	Tonnetz    [6]float64
}

// computeKey exposes the normalized chroma profile described by Bartsch and
// Wakefield (2005) and the tonal centroid projection described by Harte,
// Sandler, and Gasser (2006), "Detecting harmonic change in musical audio."
func computeKey(buffer zaudio.PCMBuffer) KeyStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return KeyStats{}
	}

	windowSize := 4096
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
	chroma := make([]float64, 12)
	frame := make([]float64, windowSize)
	power := make([]float64, windowSize/2+1)
	frames := 0

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
		accumulateChroma(chroma, current, buffer.SampleRate, fft)
		frames++

		if end == len(mono) {
			break
		}
	}

	if frames == 0 {
		return KeyStats{}
	}
	normalizeChroma(chroma)

	bestMode := ""
	bestRoot := 0
	bestScore := -1.0
	for root := 0; root < 12; root++ {
		majorScore := correlation(chroma, rotateProfile(majorProfile, root))
		if majorScore > bestScore {
			bestScore = majorScore
			bestRoot = root
			bestMode = "major"
		}
		minorScore := correlation(chroma, rotateProfile(minorProfile, root))
		if minorScore > bestScore {
			bestScore = minorScore
			bestRoot = root
			bestMode = "minor"
		}
	}

	if bestMode == "" {
		return KeyStats{}
	}

	var chromaProfile [12]float64
	copy(chromaProfile[:], chroma)

	return KeyStats{
		Key:        fmt.Sprintf("%s %s", noteClasses[bestRoot], bestMode),
		Tonic:      noteClasses[bestRoot],
		Mode:       bestMode,
		RootIndex:  bestRoot,
		Confidence: bestScore,
		Chroma:     chromaProfile,
		Tonnetz:    tonalCentroid(chroma),
	}
}

func buildKeyMetrics(stats KeyStats) map[string]float64 {
	metrics := map[string]float64{
		"key_confidence": stats.Confidence,
		"key_root_index": float64(stats.RootIndex),
	}
	for i, value := range stats.Chroma {
		metrics[fmt.Sprintf("chroma_%d", i)] = value
	}
	for i, value := range stats.Tonnetz {
		metrics[fmt.Sprintf("tonnetz_%d", i)] = value
	}
	return metrics
}

func accumulateChroma(chroma, power []float64, sampleRate int, fft *fourier.FFT) {
	for i := 1; i < len(power); i++ {
		freq := fft.Freq(i) * float64(sampleRate)
		if freq < 20 {
			continue
		}
		pitchClass := frequencyToPitchClass(freq)
		chroma[pitchClass] += power[i]
	}
}

func frequencyToPitchClass(freq float64) int {
	midi := 69.0 + 12.0*math.Log2(freq/440.0)
	pc := int(math.Round(midi)) % 12
	if pc < 0 {
		pc += 12
	}
	return pc
}

func normalizeChroma(chroma []float64) {
	var total float64
	for _, value := range chroma {
		total += value
	}
	if total == 0 {
		return
	}
	for i := range chroma {
		chroma[i] /= total
	}
}

func tonalCentroid(chroma []float64) [6]float64 {
	var tonnetz [6]float64
	for i, value := range chroma {
		pitchClass := float64(i)
		tonnetz[0] += value * math.Sin((7*math.Pi*pitchClass)/6)
		tonnetz[1] += value * math.Cos((7*math.Pi*pitchClass)/6)
		tonnetz[2] += value * math.Sin((3*math.Pi*pitchClass)/2)
		tonnetz[3] += value * math.Cos((3*math.Pi*pitchClass)/2)
		tonnetz[4] += value * math.Sin((2*math.Pi*pitchClass)/3)
		tonnetz[5] += value * math.Cos((2*math.Pi*pitchClass)/3)
	}
	return tonnetz
}

func rotateProfile(profile []float64, shift int) []float64 {
	rotated := make([]float64, len(profile))
	for i := range profile {
		rotated[(i+shift)%len(profile)] = profile[i]
	}
	return rotated
}

func correlation(a, b []float64) float64 {
	var (
		sumA, sumB, sumAB, sumASq, sumBSq float64
	)
	for i := range a {
		sumA += a[i]
		sumB += b[i]
		sumAB += a[i] * b[i]
		sumASq += a[i] * a[i]
		sumBSq += b[i] * b[i]
	}

	n := float64(len(a))
	numerator := n*sumAB - sumA*sumB
	denominator := math.Sqrt((n*sumASq - sumA*sumA) * (n*sumBSq - sumB*sumB))
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}
