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
// a one-shot event. It combines four complementary signals: spectral boundary
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

	stats := computeLoopStats(decoded.Buffer)

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
	Confidence         float64
}

func computeLoopStats(buffer zaudio.PCMBuffer) LoopStats {
	mono := mixDownMono(buffer)
	if len(mono) < 2 || buffer.SampleRate <= 0 {
		return LoopStats{}
	}

	beatStats := computeBeatStats(buffer)

	boundary := computeBoundarySimilarity(mono, buffer.SampleRate)
	tailRMS := computeTailRMSRatio(mono)
	periodicity := computePeriodicityScore(mono, buffer.SampleRate, beatStats.BeatPeriodSeconds)
	phase := computePhaseAlignment(mono, buffer.SampleRate)

	confidence := clampUnit(boundary*0.35 + tailRMS*0.30 + periodicity*0.25 + phase*0.10)

	return LoopStats{
		BoundarySimilarity: boundary,
		TailRMSRatio:       tailRMS,
		PeriodicityScore:   periodicity,
		PhaseAlignment:     phase,
		Confidence:         confidence,
	}
}

func classifyLoop(confidence float64) string {
	switch {
	case confidence >= 0.65:
		return "loop"
	case confidence < 0.35:
		return "one-shot"
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
// last 10% of the sample. Loops maintain energy at the tail; one-shots decay
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
	maxScore := 0.0

	if beatPeriodSeconds > 0 {
		// Test 1, 2, 4, and 8 bar lengths (assuming 4 beats per bar).
		for _, bars := range []float64{1, 2, 4, 8} {
			lag := int(math.Round(bars * 4 * beatPeriodSeconds * framesPerSec))
			if lag > 0 && lag < len(envelope) {
				if s := normalizedAutocorrelation(envelope, lag); s > maxScore {
					maxScore = s
				}
			}
		}
	}

	// Scan envelope-relative lags from 5% to 100% of the envelope length so
	// we catch loops at any tempo and non-tempo-based cyclic patterns.
	const steps = 20
	for i := 1; i <= steps; i++ {
		lag := int(math.Round(float64(i) / float64(steps) * float64(len(envelope))))
		if lag > 0 && lag < len(envelope) {
			if s := normalizedAutocorrelation(envelope, lag); s > maxScore {
				maxScore = s
			}
		}
	}

	// Also sweep small absolute lags (1 to steps) so very-short repeating
	// patterns are not missed by the proportional scan above.
	for lag := 1; lag <= steps && lag < len(envelope); lag++ {
		if s := normalizedAutocorrelation(envelope, lag); s > maxScore {
			maxScore = s
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
