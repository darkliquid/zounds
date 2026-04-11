package tags_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestRuleTaggerGeneratesDarkPadAndSubTags(t *testing.T) {
	t.Parallel()

	tagger := tags.NewRuleTagger()
	result := core.AnalysisResult{
		Metrics: map[string]float64{
			"spectral_centroid_hz":  200,
			"dominant_frequency_hz": 55,
			"spectral_flatness":     0.08,
			"attack_sharpness":      0.1,
			"sustain_ratio":         0.82,
			"transient_rate":        2,
			"frequency_hz":          55,
			"confidence":            0.91,
		},
	}

	got, err := tagger.Tags(context.Background(), core.Sample{}, result)
	if err != nil {
		t.Fatalf("rule tags: %v", err)
	}

	assertTagNames(t, got, "dark", "pad", "sub")
}

func TestRuleTaggerGeneratesBellGlitchAndHooverTags(t *testing.T) {
	t.Parallel()

	tagger := tags.NewRuleTagger()
	result := core.AnalysisResult{
		Metrics: map[string]float64{
			"dominant_frequency_hz": 1400,
			"spectral_flatness":     0.12,
			"sustain_ratio":         0.5,
			"confidence":            0.7,
			"spectral_flux":         0.16,
			"transient_rate":        24,
			"zero_crossing_rate":    0.11,
			"frequency_hz":          220,
		},
	}

	got, err := tagger.Tags(context.Background(), core.Sample{}, result)
	if err != nil {
		t.Fatalf("rule tags: %v", err)
	}

	assertTagNames(t, got, "bell", "glitch", "hoover")
}

func assertTagNames(t *testing.T, got []core.Tag, expected ...string) {
	t.Helper()

	seen := make(map[string]struct{}, len(got))
	for _, tag := range got {
		seen[tag.NormalizedName] = struct{}{}
	}
	for _, name := range expected {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing tag %q in %v", name, seen)
		}
	}
}
