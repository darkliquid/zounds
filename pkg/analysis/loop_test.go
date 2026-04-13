package analysis

import (
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

// ---------------------------------------------------------------------------
// computeLoopStats – integration-level tests
// ---------------------------------------------------------------------------

func TestComputeLoopStatsOnLoopLikeSignal(t *testing.T) {
	t.Parallel()

	// Constant-amplitude repeating sine: high boundary similarity, no tail decay,
	// flat RMS envelope → all loop signals should be strong.
	data := testSineBuffer(44100*2, 440, 44100)
	buffer := testPCMBuffer(44100, data)

	stats := computeLoopStats(buffer)

	if stats.BoundarySimilarity < 0.8 {
		t.Errorf("boundary_similarity: want ≥ 0.8, got %f", stats.BoundarySimilarity)
	}
	if stats.TailRMSRatio < 0.8 {
		t.Errorf("tail_rms_ratio: want ≥ 0.8, got %f", stats.TailRMSRatio)
	}
	if stats.Confidence < 0.65 {
		t.Errorf("confidence: want ≥ 0.65, got %f", stats.Confidence)
	}
	if got := classifyLoop(stats.Confidence); got != "loop" {
		t.Errorf("loop_class: want %q, got %q (confidence %f)", "loop", got, stats.Confidence)
	}
}

func TestComputeLoopStatsOnOneShotSignal(t *testing.T) {
	t.Parallel()

	// Sine in the first half, silence in the second half: boundary similarity,
	// tail RMS ratio, and phase alignment should all be near zero.
	half := 44100
	data := make([]float64, half*2)
	copy(data, testSineBuffer(half, 440, 44100))

	buffer := testPCMBuffer(44100, data)
	stats := computeLoopStats(buffer)

	if stats.TailRMSRatio > 0.1 {
		t.Errorf("tail_rms_ratio: want ≤ 0.1 for one-shot, got %f", stats.TailRMSRatio)
	}
	if stats.BoundarySimilarity > 0.2 {
		t.Errorf("boundary_similarity: want ≤ 0.2 for one-shot, got %f", stats.BoundarySimilarity)
	}
	if stats.Confidence >= 0.35 {
		t.Errorf("confidence: want < 0.35 for one-shot, got %f", stats.Confidence)
	}
	if got := classifyLoop(stats.Confidence); got != "one-shot" {
		t.Errorf("loop_class: want %q, got %q (confidence %f)", "one-shot", got, stats.Confidence)
	}
}

func TestComputeLoopStatsEmptyBuffer(t *testing.T) {
	t.Parallel()

	stats := computeLoopStats(zaudio.PCMBuffer{})
	if stats != (LoopStats{}) {
		t.Errorf("expected zero LoopStats for empty buffer, got %+v", stats)
	}
}

func TestComputeLoopStatsNearSilence(t *testing.T) {
	t.Parallel()

	buffer := testPCMBuffer(44100, make([]float64, 44100))
	stats := computeLoopStats(buffer)

	// overallRMS = 0 → tail ratio undefined → 0; confidence should be low.
	if stats.TailRMSRatio != 0 {
		t.Errorf("tail_rms_ratio: want 0 for silent buffer, got %f", stats.TailRMSRatio)
	}
	if stats.Confidence > 0.35 {
		t.Errorf("confidence: want ≤ 0.35 for silent buffer, got %f", stats.Confidence)
	}
}

// ---------------------------------------------------------------------------
// computeBoundarySimilarity
// ---------------------------------------------------------------------------

func TestComputeBoundarySimilarityIdenticalWindows(t *testing.T) {
	t.Parallel()

	// Signal that repeats exactly once so head == tail at the window boundaries.
	segment := testSineBuffer(2048, 440, 44100)
	data := append(append([]float64(nil), segment...), segment...)

	score := computeBoundarySimilarity(data, 44100)
	if score < 0.99 {
		t.Errorf("want ≥ 0.99 for repeated segment, got %f", score)
	}
}

func TestComputeBoundarySimilaritySilentTail(t *testing.T) {
	t.Parallel()

	head := testSineBuffer(2048, 440, 44100)
	tail := make([]float64, 2048) // silence
	data := append(append([]float64(nil), head...), tail...)

	score := computeBoundarySimilarity(data, 44100)
	if score > 0.05 {
		t.Errorf("want ≤ 0.05 for silent tail, got %f", score)
	}
}

func TestComputeBoundarySimilarityEmptyBuffer(t *testing.T) {
	t.Parallel()

	if score := computeBoundarySimilarity(nil, 44100); score != 0 {
		t.Errorf("want 0 for nil input, got %f", score)
	}
}

// ---------------------------------------------------------------------------
// computeTailRMSRatio
// ---------------------------------------------------------------------------

func TestComputeTailRMSRatioUniform(t *testing.T) {
	t.Parallel()

	data := testSineBuffer(44100, 440, 44100)
	ratio := computeTailRMSRatio(data)
	if ratio < 0.9 {
		t.Errorf("want ≥ 0.9 for uniform-amplitude signal, got %f", ratio)
	}
}

func TestComputeTailRMSRatioDecays(t *testing.T) {
	t.Parallel()

	n := 44100
	data := make([]float64, n)
	for i := range data {
		amplitude := 1.0 - float64(i)/float64(n)
		data[i] = amplitude * math.Sin(2*math.Pi*440*float64(i)/44100)
	}

	ratio := computeTailRMSRatio(data)
	if ratio > 0.2 {
		t.Errorf("want ≤ 0.2 for linearly-decaying signal, got %f", ratio)
	}
}

func TestComputeTailRMSRatioEmpty(t *testing.T) {
	t.Parallel()

	if r := computeTailRMSRatio(nil); r != 0 {
		t.Errorf("want 0 for nil input, got %f", r)
	}
}

// ---------------------------------------------------------------------------
// computePeriodicityScore
// ---------------------------------------------------------------------------

func TestComputePeriodicityScoreConstantEnvelope(t *testing.T) {
	t.Parallel()

	// Constant-amplitude sine → flat RMS envelope → autocorrelation = 1 at
	// every lag.
	data := testSineBuffer(44100*2, 440, 44100)
	score := computePeriodicityScore(data, 44100, 0)
	if score < 0.9 {
		t.Errorf("want ≥ 0.9 for constant-envelope signal, got %f", score)
	}
}

func TestComputePeriodicityScoreImpulseAtStart(t *testing.T) {
	t.Parallel()

	// Single impulse followed by silence → all envelope frames after index 0
	// are zero, so autocorrelation at every lag ≥ 1 is undefined (energyB = 0)
	// → score = 0.
	data := make([]float64, 44100)
	data[0] = 1.0

	score := computePeriodicityScore(data, 44100, 0)
	if score > 0.3 {
		t.Errorf("want ≤ 0.3 for single-impulse signal, got %f", score)
	}
}

func TestComputePeriodicityScoreEmpty(t *testing.T) {
	t.Parallel()

	if s := computePeriodicityScore(nil, 44100, 0); s != 0 {
		t.Errorf("want 0 for nil input, got %f", s)
	}
}

// ---------------------------------------------------------------------------
// computePhaseAlignment
// ---------------------------------------------------------------------------

func TestComputePhaseAlignmentIdenticalFrames(t *testing.T) {
	t.Parallel()

	// Signal constructed so that head window == tail window. FFT of identical
	// frames yields identical coefficients and therefore zero phase difference.
	segment := testSineBuffer(2048, 440, 44100)
	data := append(append([]float64(nil), segment...), segment...)

	score := computePhaseAlignment(data, 44100)
	if score < 0.99 {
		t.Errorf("want ≥ 0.99 for identical frames, got %f", score)
	}
}

func TestComputePhaseAlignmentSilentTail(t *testing.T) {
	t.Parallel()

	head := testSineBuffer(2048, 440, 44100)
	tail := make([]float64, 2048) // silence
	data := append(append([]float64(nil), head...), tail...)

	// Tail has negligible energy at the dominant bin → phase is undefined → 0.
	score := computePhaseAlignment(data, 44100)
	if score != 0 {
		t.Errorf("want 0 for silent tail, got %f", score)
	}
}

func TestComputePhaseAlignmentEmpty(t *testing.T) {
	t.Parallel()

	if s := computePhaseAlignment(nil, 44100); s != 0 {
		t.Errorf("want 0 for nil input, got %f", s)
	}
}

// ---------------------------------------------------------------------------
// classifyLoop
// ---------------------------------------------------------------------------

func TestClassifyLoop(t *testing.T) {
	t.Parallel()

	cases := []struct {
		confidence float64
		want       string
	}{
		{1.0, "loop"},
		{0.65, "loop"},
		{0.64, "ambiguous"}, // just below the 0.65 loop threshold
		{0.5, "ambiguous"},
		{0.35, "ambiguous"},
		{0.349, "one-shot"},
		{0.0, "one-shot"},
	}

	for _, tc := range cases {
		got := classifyLoop(tc.confidence)
		if got != tc.want {
			t.Errorf("classifyLoop(%f) = %q, want %q", tc.confidence, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// boundaryWindowSize
// ---------------------------------------------------------------------------

func TestBoundaryWindowSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		signalLen int
		want      int
	}{
		{8192, 2048},
		{4096, 2048},
		{4095, 1024},
		{2048, 1024},
		{4, 2},
		{3, 0}, // 2*2=4 > 3 → w halves to 1 < 2 → 0
		{0, 0},
	}

	for _, tc := range cases {
		got := boundaryWindowSize(tc.signalLen)
		if got != tc.want {
			t.Errorf("boundaryWindowSize(%d) = %d, want %d", tc.signalLen, got, tc.want)
		}
	}
}
