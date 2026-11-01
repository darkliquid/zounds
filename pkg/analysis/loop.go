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

const loopAnalyzerVersion = "0.1.0"

// LoopAnalyzer detects whether a sample is intended for seamless looping or is
// a oneshot event. It combines four complementary signals: spectral boundary
// similarity, tail energy sustain, RMS-envelope periodicity, and dominant-
// frequency phase alignment at the loop point.
type LoopAnalyzer struct {
	registry *zaudio.Registry
}

func NewLoopAnalyzer(registry *zaudio.Registry) (*LoopAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}
	return &LoopAnalyzer{registry: registry}, nil
}

func (a *LoopAnalyzer) Name() string {
	return "loop"
}

func (a *LoopAnalyzer) Version() string {
	return loopAnalyzerVersion
}

func (a *LoopAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("loop analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze loop for %q: %w", sample.Path, err)
	}

	// Check container loop flags (smpl, acid, basc)
	containerLoopClass := ""
	if sample.Metadata != nil {
		if lc, ok := sample.Metadata["embedded.loop_class"]; ok && lc != "" {
			containerLoopClass = lc
		} else if lc, ok := sample.Metadata["loop_class"]; ok && lc != "" {
			containerLoopClass = lc
		}
	}
	if containerLoopClass == "" && sample.Path != "" {
		if embedded, err := ReadEmbeddedMetadata(sample.Path); err == nil {
			if lc, ok := embedded.Values["loop_class"]; ok && lc != "" {
				containerLoopClass = lc
			}
		}
	}

	stats := computeLoopStats(decoded.Buffer)
	switch containerLoopClass {
	case "loop":
		stats.Confidence = 1.0
	case "oneshot":
		stats.Confidence = 0.0
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"loop_boundary_similarity": stats.BoundarySimilarity,
			"loop_tail_rms_ratio":      stats.TailRMSRatio,
			"loop_periodicity_score":   stats.PeriodicityScore,
			"loop_phase_alignment":     stats.PhaseAlignment,
			"loop_bar_alignment":       stats.BarAlignment,
			"loop_confidence":          stats.Confidence,
		},
		Attributes: map[string]string{
			"loop_class":     classifyLoop(stats.Confidence),
			"channel_layout": channelLayout(decoded.Buffer.Channels),
		},
	}, nil
}

// LoopStats holds the intermediate and final scores produced by loop detection.
type LoopStats struct {
	BoundarySimilarity float64
	TailRMSRatio       float64
	PeriodicityScore   float64
	PhaseAlignment     float64
	BarAlignment       float64
	Confidence         float64
}

func computeLoopStats(buffer zaudio.PCMBuffer) LoopStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return LoopStats{}
	}

	duration := buffer.Duration().Seconds()
	beatStats := computeBeatStats(buffer)

	boundary := computeBoundarySimilarity(mono, buffer.SampleRate)
	tailRMS := computeTailRMSRatio(mono)
	periodicity := computePeriodicityScore(mono, buffer.SampleRate, beatStats.BeatPeriodSeconds)
	phase := computePhaseAlignment(mono, buffer.SampleRate)
	barAlignment := computeBarAlignment(duration, beatStats)

	// Decision model:
	// A short sample (< 0.8s) cannot be a repeating musical bar loop.
	// Similarly, if the tail has decayed to near-silence (tailRMS < 0.10),
	// it is a one-shot event with a decay tail.
	if duration < 0.8 || tailRMS < 0.10 {
		conf := math.Min(0.25, tailRMS*2.0)
		return LoopStats{
			BoundarySimilarity: boundary,
			TailRMSRatio:       tailRMS,
			PeriodicityScore:   periodicity,
			PhaseAlignment:     phase,
			BarAlignment:       barAlignment,
			Confidence:         conf,
		}
	}

	// Seamless loop score (continuous drones, pads, synths, seamless basslines)
	seamlessScore := 0.40*boundary + 0.35*tailRMS + 0.15*phase + 0.10*periodicity
	// Rhythmic loop score (drum breaks, percussion loops, guitar grooves, gapped breaks)
	rhythmicScore := 0.50*barAlignment + 0.30*periodicity + 0.20*tailRMS

	loopEvidence := math.Max(seamlessScore, rhythmicScore)
	blended := 0.60*loopEvidence + 0.40*(0.25*boundary+0.25*tailRMS+0.25*periodicity+0.25*barAlignment)
	confidence := clampUnit(blended)

	return LoopStats{
		BoundarySimilarity: boundary,
		TailRMSRatio:       tailRMS,
		PeriodicityScore:   periodicity,
		PhaseAlignment:     phase,
		BarAlignment:       barAlignment,
		Confidence:         confidence,
	}
}

func computeBarAlignment(duration float64, beatStats BeatStats) float64 {
	if duration < 0.8 {
		return 0
	}

	// 1. If tempo was detected with high confidence, check alignment with detected BPM
	if beatStats.Confidence >= 0.50 && beatStats.BPM > 0 {
		beatPeriod := 60.0 / beatStats.BPM
		numBeats := duration / beatPeriod

		targets := []float64{2, 3, 4, 6, 8, 12, 16, 24, 32, 48, 64}
		minDiff := math.MaxFloat64
		for _, target := range targets {
			diff := math.Abs(numBeats - target)
			if diff < minDiff {
				minDiff = diff
			}
		}

		if minDiff <= 0.10 {
			return clampUnit(1.0 - minDiff/0.10)
		}
	}

	// 2. Grid tempo check: check if duration matches an exact integer BPM for standard bar counts
	barCounts := []float64{2, 4, 8, 16, 32}
	for _, bars := range barCounts {
		impliedBPM := bars * 60.0 / duration
		if impliedBPM >= 60.0 && impliedBPM <= 180.0 {
			rounded := math.Round(impliedBPM)
			diff := math.Abs(impliedBPM - rounded)
			if diff < 0.02 {
				return 0.85
			}
		}
	}

	return 0
}

func classifyLoop(confidence float64) string {
	switch {
	case confidence >= 0.65:
		return "loop"
	case confidence < 0.35:
		return "oneshot"
	default:
		return "ambiguous"
	}
}

// computeBoundarySimilarity returns the cosine similarity between the magnitude
// spectra of the head and tail windows. A seamless loop has spectrally similar
// start and end; a one-shot typically ends in silence or a very different
// spectral state.
func computeBoundarySimilarity(mono []float64, sampleRate int) float64 {
	windowSize := boundaryWindowSize(len(mono))
	if windowSize < 2 {
		return 0
	}

	head := make([]float64, windowSize)
	copy(head, mono[:windowSize])
	applyHannWindow(head)

	tail := make([]float64, windowSize)
	copy(tail, mono[len(mono)-windowSize:])
	applyHannWindow(tail)

	fft := fourier.NewFFT(windowSize)
	headMags := magnitudeSpectrum(fft.Coefficients(nil, head))
	tailMags := magnitudeSpectrum(fft.Coefficients(nil, tail))

	return cosineSimilarity(headMags, tailMags)
}

// computeTailRMSRatio returns tail_rms / overall_rms where the tail is the
// last 10% of the sample. Loops maintain energy at the tail; oneshots decay
// to near-silence.
func computeTailRMSRatio(mono []float64) float64 {
	if len(mono) == 0 {
		return 0
	}

	overallRMS := loopRMSOf(mono)
	if overallRMS == 0 {
		return 0
	}

	tailLen := len(mono) / 10
	if tailLen < 1 {
		tailLen = 1
	}

	tailRMS := loopRMSOf(mono[len(mono)-tailLen:])
	return clampUnit(tailRMS / overallRMS)
}

// computePeriodicityScore measures the strongest normalised autocorrelation of
// the RMS envelope at musically plausible phrase-length lags. A high score
// indicates the energy pattern repeats periodically, which is characteristic
// of a designed loop. beatPeriodSeconds is used to test musically meaningful
// bar-length lags; a range-based scan runs in parallel to catch loops at any
// tempo.
func computePeriodicityScore(mono []float64, sampleRate int, beatPeriodSeconds float64) float64 {
	if len(mono) < 2 || sampleRate <= 0 {
		return 0
	}

	const envWindowSize = 1024
	envelope := loopRMSEnvelope(mono, envWindowSize)
	if len(envelope) < 2 {
		return 0
	}

	framesPerSec := float64(sampleRate) / float64(envWindowSize)
	// Minimum lag corresponding to the fastest plausible musical tempo (~240 BPM = 0.25s)
	minLagFrames := int(math.Ceil(0.25 * framesPerSec))
	if minLagFrames < 4 {
		minLagFrames = 4
	}

	maxScore := 0.0

	if beatPeriodSeconds > 0 {
		// Test 1, 2, 4, and 8 bar lengths (assuming 4 beats per bar).
		for _, bars := range []float64{1, 2, 4, 8} {
			lag := int(math.Round(bars * 4 * beatPeriodSeconds * framesPerSec))
			if lag >= minLagFrames && lag < len(envelope) {
				if s := normalizedAutocorrelation(envelope, lag); s > maxScore {
					maxScore = s
				}
			}
		}
	}

	// Scan envelope-relative lags (from 20% to 100% of the envelope length),
	// strictly bounded above minLagFrames to avoid smooth-envelope false positives.
	const steps = 20
	for i := 4; i <= steps; i++ {
		lag := int(math.Round(float64(i) / float64(steps) * float64(len(envelope))))
		if lag >= minLagFrames && lag < len(envelope) {
			if s := normalizedAutocorrelation(envelope, lag); s > maxScore {
				maxScore = s
			}
		}
	}

	return clampUnit(maxScore)
}

// computePhaseAlignment compares the instantaneous phase of the dominant
// spectral bin between the head and tail windows. A loop designed for seamless
// playback will have closely matched phases at the loop boundary. If the tail
// window has negligible energy relative to the head, the phase is undefined and
// 0 is returned.
func computePhaseAlignment(mono []float64, sampleRate int) float64 {
	windowSize := boundaryWindowSize(len(mono))
	if windowSize < 2 {
		return 0
	}

	head := make([]float64, windowSize)
	copy(head, mono[:windowSize])
	applyHannWindow(head)

	tail := make([]float64, windowSize)
	copy(tail, mono[len(mono)-windowSize:])
	applyHannWindow(tail)

	fft := fourier.NewFFT(windowSize)
	headCoeffs := fft.Coefficients(nil, head)
	tailCoeffs := fft.Coefficients(nil, tail)

	headMags := magnitudeSpectrum(headCoeffs)

	// Find the dominant bin (skip DC at index 0).
	dominantBin := 1
	maxMag := 0.0
	for i := 1; i < len(headMags); i++ {
		if headMags[i] > maxMag {
			maxMag = headMags[i]
			dominantBin = i
		}
	}

	if maxMag == 0 || dominantBin >= len(tailCoeffs) {
		return 0
	}

	// If the tail has negligible energy at the dominant bin, the phase is
	// undefined; return 0 rather than a spurious alignment score.
	tailMag := cmplxAbs(tailCoeffs[dominantBin])
	if tailMag < maxMag*0.05 {
		return 0
	}

	headPhase := math.Atan2(imag(headCoeffs[dominantBin]), real(headCoeffs[dominantBin]))
	tailPhase := math.Atan2(imag(tailCoeffs[dominantBin]), real(tailCoeffs[dominantBin]))

	// Wrap the phase difference into [0, π].
	diff := math.Mod(math.Abs(headPhase-tailPhase), 2*math.Pi)
	if diff > math.Pi {
		diff = 2*math.Pi - diff
	}

	return 1 - diff/math.Pi
}

// boundaryWindowSize returns the largest power-of-two window size up to 2048
// that fits within half the signal length, ensuring head and tail windows never
// overlap.
func boundaryWindowSize(signalLen int) int {
	const maxWindow = 2048
	w := maxWindow
	for w > 1 && w*2 > signalLen {
		w >>= 1
	}
	if w < 2 {
		return 0
	}
	return w
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return clampUnit(dot / denom)
}

func loopRMSOf(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sumSq float64
	for _, s := range samples {
		sumSq += s * s
	}
	return math.Sqrt(sumSq / float64(len(samples)))
}

func loopRMSEnvelope(samples []float64, windowSize int) []float64 {
	if len(samples) == 0 || windowSize <= 0 {
		return nil
	}
	out := make([]float64, 0, len(samples)/windowSize+1)
	for start := 0; start < len(samples); start += windowSize {
		end := minInt(len(samples), start+windowSize)
		var sumSq float64
		for _, s := range samples[start:end] {
			sumSq += s * s
		}
		out = append(out, math.Sqrt(sumSq/float64(end-start)))
	}
	return out
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
