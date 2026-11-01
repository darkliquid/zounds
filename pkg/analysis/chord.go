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

const chordAnalyzerVersion = "0.1.0"

// ChordAnalyzer performs log-frequency harmonic-deconvolved chromagram template
// matching to detect musical chords (triads, 7ths, sus chords) while distinguishing
// true polyphonic chords from single-note acoustic overtones.
type ChordAnalyzer struct {
	registry *zaudio.Registry
}

func NewChordAnalyzer(registry *zaudio.Registry) (*ChordAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}
	return &ChordAnalyzer{registry: registry}, nil
}

func (a *ChordAnalyzer) Name() string {
	return "chord"
}

func (a *ChordAnalyzer) Version() string {
	return chordAnalyzerVersion
}

type ChordStats struct {
	Detected   bool
	Chord      string
	Root       string
	Quality    string
	RootIndex  int
	Confidence float64
	Chroma     [12]float64
}

func (a *ChordAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("chord analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze chord for %q: %w", sample.Path, err)
	}

	stats := computeChord(decoded.Buffer)

	metrics := map[string]float64{
		"chord_confidence": stats.Confidence,
	}
	if stats.Detected {
		metrics["chord_root_index"] = float64(stats.RootIndex)
	}

	attributes := map[string]string{
		"channel_layout": channelLayout(decoded.Buffer.Channels),
	}
	if stats.Detected {
		attributes["chord"] = stats.Chord
		attributes["chord_root"] = stats.Root
		attributes["chord_quality"] = stats.Quality
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics:     metrics,
		Attributes:  attributes,
	}, nil
}

type chordTemplate struct {
	quality   string
	intervals []int
	weights   []float64
}

var chordTemplates = []chordTemplate{
	{quality: "major", intervals: []int{0, 4, 7}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "minor", intervals: []int{0, 3, 7}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "dim", intervals: []int{0, 3, 6}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "aug", intervals: []int{0, 4, 8}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "sus2", intervals: []int{0, 2, 7}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "sus4", intervals: []int{0, 5, 7}, weights: []float64{1.0, 1.0, 1.0}},
	{quality: "7", intervals: []int{0, 4, 7, 10}, weights: []float64{1.0, 1.0, 1.0, 0.9}},
	{quality: "maj7", intervals: []int{0, 4, 7, 11}, weights: []float64{1.0, 1.0, 1.0, 0.9}},
	{quality: "min7", intervals: []int{0, 3, 7, 10}, weights: []float64{1.0, 1.0, 1.0, 0.9}},
	{quality: "dim7", intervals: []int{0, 3, 6, 9}, weights: []float64{1.0, 1.0, 1.0, 0.9}},
	{quality: "m7b5", intervals: []int{0, 3, 6, 10}, weights: []float64{1.0, 1.0, 1.0, 0.9}},
}

func computeChord(buffer zaudio.PCMBuffer) ChordStats {
	mono := mixDownMono(buffer)
	if len(mono) < 1024 || buffer.SampleRate <= 0 {
		return ChordStats{}
	}

	windowSize := 8192
	if len(mono) < windowSize {
		windowSize = nextPowerOfTwo(len(mono))
		if windowSize < 1024 {
			windowSize = len(mono)
		}
	}
	if windowSize < 256 {
		return ChordStats{}
	}

	hopSize := windowSize / 2
	if hopSize == 0 {
		hopSize = 1
	}

	fft := fourier.NewFFT(windowSize)
	frame := make([]float64, windowSize)
	power := make([]float64, windowSize/2+1)

	// Semitone energies from MIDI 36 (C2, ~65.4 Hz) to 96 (C7, ~2093 Hz)
	const minMidi = 36
	const maxMidi = 96
	const numSemitones = maxMidi - minMidi + 1
	semitoneEnergy := make([]float64, numSemitones)
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
		powerSpectrumInto(coeff, power)
		frames++

		// Accumulate energy per semitone
		for i := 1; i < len(power); i++ {
			freq := fft.Freq(i) * float64(buffer.SampleRate)
			if freq < 40 || freq > 4500 {
				continue
			}
			midi := int(math.Round(69.0 + 12.0*math.Log2(freq/440.0)))
			if midi >= minMidi && midi <= maxMidi {
				semitoneEnergy[midi-minMidi] += power[i]
			}
		}

		if end == len(mono) {
			break
		}
	}

	if frames == 0 {
		return ChordStats{}
	}

	for i := range semitoneEnergy {
		semitoneEnergy[i] /= float64(frames)
	}

	// Harmonic deconvolution (attenuate acoustic overtone spillover)
	cleanSemitones := make([]float64, len(semitoneEnergy))
	copy(cleanSemitones, semitoneEnergy)

	for m := minMidi; m <= maxMidi-19; m++ {
		e := cleanSemitones[m-minMidi]
		if e <= 0 {
			continue
		}
		// 3rd harmonic (+19 semitones, octave + fifth)
		h3 := m + 19 - minMidi
		if h3 < len(cleanSemitones) {
			cleanSemitones[h3] = math.Max(0, cleanSemitones[h3]-0.45*e)
		}
		// 5th harmonic (+28 semitones, 2 octaves + major third)
		h5 := m + 28 - minMidi
		if h5 < len(cleanSemitones) {
			cleanSemitones[h5] = math.Max(0, cleanSemitones[h5]-0.25*e)
		}
	}

	// Fold clean semitones into 12-bin chroma
	chroma := make([]float64, 12)
	for m := minMidi; m <= maxMidi; m++ {
		pc := m % 12
		chroma[pc] += cleanSemitones[m-minMidi]
	}

	var maxVal, totalVal float64
	for _, v := range chroma {
		totalVal += v
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal <= 1e-12 || totalVal <= 0 {
		return ChordStats{}
	}

	for i := range chroma {
		chroma[i] /= totalVal
	}

	// Count prominent pitch classes to distinguish monophonic vs polyphonic vs noise
	prominentCount := 0
	normMax := maxVal / totalVal
	for _, v := range chroma {
		if v >= 0.35*normMax {
			prominentCount++
		}
	}

	// Check for unpitched noise: chroma variance is very low or almost uniform
	var minVal = 1.0
	for _, v := range chroma {
		if v < minVal {
			minVal = v
		}
	}
	if minVal > 0.05 && (minVal/normMax) > 0.65 {
		// Uniform distribution across all pitch classes = noise
		return ChordStats{Confidence: 0.15}
	}

	// If fewer than 2 prominent pitch classes exist, it's a single monophonic note
	if prominentCount < 2 {
		return ChordStats{Confidence: 0.25}
	}

	// Template matching across 12 roots and all chord templates
	bestScore := -1.0
	bestRoot := -1
	bestQuality := ""

	for root := 0; root < 12; root++ {
		for _, tmpl := range chordTemplates {
			score := matchChordTemplate(chroma, root, tmpl)
			if score > bestScore {
				bestScore = score
				bestRoot = root
				bestQuality = tmpl.quality
			}
		}
	}

	var chromaArr [12]float64
	copy(chromaArr[:], chroma)

	// Confidence threshold for chord detection
	const confidenceThreshold = 0.58
	if bestScore >= confidenceThreshold && bestRoot >= 0 {
		rootName := noteClasses[bestRoot]
		chordName := fmt.Sprintf("%s %s", rootName, bestQuality)
		return ChordStats{
			Detected:   true,
			Chord:      chordName,
			Root:       rootName,
			Quality:    bestQuality,
			RootIndex:  bestRoot,
			Confidence: math.Min(1.0, bestScore),
			Chroma:     chromaArr,
		}
	}

	return ChordStats{
		Detected:   false,
		Confidence: math.Max(0.1, bestScore*0.7),
		Chroma:     chromaArr,
	}
}

func matchChordTemplate(chroma []float64, root int, tmpl chordTemplate) float64 {
	chordToneSet := make([]bool, 12)
	for _, interval := range tmpl.intervals {
		pc := (root + interval) % 12
		chordToneSet[pc] = true
	}

	// Check presence of essential chord tones
	var chordEnergy, nonChordEnergy float64
	for pc := 0; pc < 12; pc++ {
		if chordToneSet[pc] {
			chordEnergy += chroma[pc]
		} else {
			nonChordEnergy += chroma[pc]
		}
	}

	// Root and all chord tones must be present
	for _, interval := range tmpl.intervals {
		pc := (root + interval) % 12
		if chroma[pc] < 0.04 {
			return 0
		}
	}

	// Dot product with template weights
	var dot, normC, normT float64
	for i, interval := range tmpl.intervals {
		pc := (root + interval) % 12
		w := tmpl.weights[i]
		dot += chroma[pc] * w
		normT += w * w
	}
	for _, c := range chroma {
		normC += c * c
	}

	if normC <= 0 || normT <= 0 {
		return 0
	}

	cosine := dot / (math.Sqrt(normC) * math.Sqrt(normT))

	// Penalize energy in non-chord tones
	penalty := nonChordEnergy / (chordEnergy + nonChordEnergy + 1e-9)
	score := cosine * (1.0 - 0.75*penalty)

	return math.Max(0, score)
}
