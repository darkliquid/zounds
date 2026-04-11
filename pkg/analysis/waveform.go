package analysis

import (
	"context"
	"fmt"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const waveformAnalyzerVersion = "0.1.0"

type WaveformAnalyzer struct {
	registry *zaudio.Registry
}

func NewWaveformAnalyzer(registry *zaudio.Registry) (*WaveformAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
	}

	return &WaveformAnalyzer{registry: registry}, nil
}

func (a *WaveformAnalyzer) Name() string {
	return "waveform"
}

func (a *WaveformAnalyzer) Version() string {
	return waveformAnalyzerVersion
}

func (a *WaveformAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("waveform analyzer is not initialized")
	}

	decoded, err := decodeSample(ctx, a.registry, sample)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze waveform for %q: %w", sample.Path, err)
	}

	spectral := computeSpectral(decoded.Buffer)
	dynamics := computeDynamics(decoded.Buffer)
	pitch := computePitch(decoded.Buffer)
	classification, texture := classifyWaveform(spectral, dynamics, pitch)

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"waveform_tonal_score":      tonalScore(spectral, dynamics, pitch),
			"waveform_percussive_score": percussiveScore(spectral, dynamics, pitch),
			"waveform_noise_score":      noiseScore(spectral, dynamics, pitch),
		},
		Attributes: map[string]string{
			"waveform_class":   classification,
			"waveform_texture": texture,
		},
	}, nil
}

func classifyWaveform(spectral SpectralStats, dynamics DynamicsStats, pitch PitchStats) (string, string) {
	tonal := tonalScore(spectral, dynamics, pitch)
	percussive := percussiveScore(spectral, dynamics, pitch)
	noise := noiseScore(spectral, dynamics, pitch)

	classification := "mixed"
	best := tonal
	if percussive > best {
		classification = "percussive"
		best = percussive
	} else {
		classification = "tonal"
	}
	if noise > best {
		classification = "noise"
		best = noise
	}

	texture := "hybrid"
	switch {
	case classification == "noise":
		texture = "noisy"
	case classification == "percussive" || dynamics.AttackSharpness >= 0.5 || dynamics.TransientRate >= 20:
		texture = "transient"
	case spectral.Flatness >= 0.4:
		texture = "noisy"
	case dynamics.SustainRatio >= 0.65:
		texture = "sustained"
	}

	return classification, texture
}

func tonalScore(spectral SpectralStats, dynamics DynamicsStats, pitch PitchStats) float64 {
	score := 0.0
	if pitch.Confidence > 0.5 {
		score += 0.5
	}
	if spectral.Flatness < 0.25 {
		score += 0.3
	}
	if dynamics.SustainRatio > 0.45 {
		score += 0.2
	}
	return score
}

func percussiveScore(spectral SpectralStats, dynamics DynamicsStats, pitch PitchStats) float64 {
	score := 0.0
	if dynamics.AttackSharpness > 0.5 {
		score += 0.4
	}
	if dynamics.SustainRatio < 0.4 {
		score += 0.3
	}
	if dynamics.TransientRate > 20 {
		score += 0.2
	}
	if spectral.Flux > 0.08 {
		score += 0.1
	}
	if pitch.Confidence < 0.35 {
		score += 0.1
	}
	return score
}

func noiseScore(spectral SpectralStats, dynamics DynamicsStats, pitch PitchStats) float64 {
	score := 0.0
	if spectral.Flatness > 0.45 {
		score += 0.5
	}
	if spectral.ZeroCrossingRate > 0.1 {
		score += 0.2
	}
	if pitch.Confidence < 0.3 {
		score += 0.2
	}
	if dynamics.SustainRatio < 0.5 {
		score += 0.1
	}
	return score
}
