package analysis

import (
	"context"
	"math"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
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
		t.Errorf("tail_rms_ratio: want ≤ 0.1 for oneshot, got %f", stats.TailRMSRatio)
	}
	if stats.BoundarySimilarity > 0.2 {
		t.Errorf("boundary_similarity: want ≤ 0.2 for oneshot, got %f", stats.BoundarySimilarity)
	}
	if stats.Confidence >= 0.35 {
		t.Errorf("confidence: want < 0.35 for oneshot, got %f", stats.Confidence)
	}
	if got := classifyLoop(stats.Confidence); got != "oneshot" {
		t.Errorf("loop_class: want %q, got %q (confidence %f)", "oneshot", got, stats.Confidence)
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
		{0.349, "oneshot"},
		{0.0, "oneshot"},
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

func TestComputeLoopStatsGappedDrumLoop(t *testing.T) {
	t.Parallel()

	// 2-bar drum loop at 120 BPM = 4.0 seconds at 44.1 kHz.
	// Starts with a kick transient at t=0, has beat hits every 0.5s,
	// but the last 0.2s before 4.0s has low-level ambient decay (gapped tail),
	// meaning head (kick transient) != tail (ambient decay).
	sampleRate := 44100
	duration := 4.0
	frames := int(duration * float64(sampleRate))
	data := make([]float64, frames)

	// Hits at beat intervals: 0.0s, 0.5s, 1.0s, 1.5s, 2.0s, 2.5s, 3.0s, 3.5s
	beatInterval := int(0.5 * float64(sampleRate))
	for beat := 0; beat < 8; beat++ {
		start := beat * beatInterval
		for i := 0; i < 2000 && start+i < frames; i++ {
			tSec := float64(i) / float64(sampleRate)
			// Kick hit on downbeats, snare/hat on backbeats
			freq := 60.0
			if beat%2 == 1 {
				freq = 200.0
			}
			data[start+i] += math.Sin(2*math.Pi*freq*tSec) * math.Exp(-tSec*25.0)
		}
	}
	// Add low-level room tone / cymbal decay throughout, including tail
	for i := range data {
		tSec := float64(i) / float64(sampleRate)
		data[i] += 0.05 * math.Sin(2*math.Pi*8000*tSec) * (0.8 + 0.2*math.Sin(2*math.Pi*2*tSec))
	}

	buffer := testPCMBuffer(sampleRate, data)
	stats := computeLoopStats(buffer)

	if stats.BarAlignment < 0.70 {
		t.Errorf("expected high bar alignment >= 0.70, got %f", stats.BarAlignment)
	}
	if stats.Confidence < 0.65 {
		t.Errorf("expected confidence >= 0.65 (loop), got %f (boundary=%f, tailRMS=%f, barAlign=%f)",
			stats.Confidence, stats.BoundarySimilarity, stats.TailRMSRatio, stats.BarAlignment)
	}
	if got := classifyLoop(stats.Confidence); got != "loop" {
		t.Errorf("loop_class: want %q, got %q (confidence %f)", "loop", got, stats.Confidence)
	}
}

func TestComputeLoopStatsShortStabNotLoop(t *testing.T) {
	t.Parallel()

	// Short 0.3s synth stab / kick (duration < 0.8s)
	sampleRate := 44100
	duration := 0.3
	frames := int(duration * float64(sampleRate))
	data := make([]float64, frames)
	for i := range data {
		tSec := float64(i) / float64(sampleRate)
		data[i] = math.Sin(2*math.Pi*150*tSec) * math.Exp(-tSec*20.0)
	}

	buffer := testPCMBuffer(sampleRate, data)
	stats := computeLoopStats(buffer)

	if stats.Confidence >= 0.35 {
		t.Errorf("expected low confidence < 0.35 on short stab, got %f", stats.Confidence)
	}
	if got := classifyLoop(stats.Confidence); got != "oneshot" {
		t.Errorf("loop_class: want %q, got %q (confidence %f)", "oneshot", got, stats.Confidence)
	}
}

func TestLoopAnalyzerContainerMetadata(t *testing.T) {
	t.Parallel()

	analyzer, err := NewLoopAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewLoopAnalyzer: %v", err)
	}

	buf := testPCMBuffer(44100, testSineBuffer(44100, 440, 44100))
	ctx := ContextWithDecodedSample(context.Background(), "test.wav", zaudio.DecodeResult{
		Buffer: buf,
		Info:   zaudio.StreamInfo{SampleRate: 44100, Channels: 1, BitDepth: 16, Format: core.FormatWAV},
	})

	// 1. Explicit loop container metadata
	resLoop, err := analyzer.Analyze(ctx, core.Sample{
		Path: "test.wav",
		Metadata: map[string]string{
			"embedded.loop_class": "loop",
		},
	})
	if err != nil {
		t.Fatalf("Analyze loop: %v", err)
	}
	if resLoop.Metrics["loop_confidence"] != 1.0 {
		t.Errorf("expected loop_confidence 1.0 from container metadata, got %f", resLoop.Metrics["loop_confidence"])
	}
	if resLoop.Attributes["loop_class"] != "loop" {
		t.Errorf("expected loop_class 'loop', got %q", resLoop.Attributes["loop_class"])
	}

	// 2. Explicit oneshot container metadata
	resOneshot, err := analyzer.Analyze(ctx, core.Sample{
		Path: "test.wav",
		Metadata: map[string]string{
			"embedded.loop_class": "oneshot",
		},
	})
	if err != nil {
		t.Fatalf("Analyze oneshot: %v", err)
	}
	if resOneshot.Metrics["loop_confidence"] != 0.0 {
		t.Errorf("expected loop_confidence 0.0 from container metadata, got %f", resOneshot.Metrics["loop_confidence"])
	}
	if resOneshot.Attributes["loop_class"] != "oneshot" {
		t.Errorf("expected loop_class 'oneshot', got %q", resOneshot.Attributes["loop_class"])
	}
}
