package tags_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestRuleTaggerGeneratesPadAndSubTags(t *testing.T) {
	t.Parallel()

	tagger, err := tags.NewRuleTagger()
	if err != nil {
		t.Fatalf("new rule tagger: %v", err)
	}
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

	assertTagNames(t, got, "pad", "sub")
}

func TestRuleTaggerGeneratesBellGlitchAndHooverTags(t *testing.T) {
	t.Parallel()

	tagger, err := tags.NewRuleTagger()
	if err != nil {
		t.Fatalf("new rule tagger: %v", err)
	}
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

func TestRuleTaggerLoadsCustomExprRules(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "rules.json")
	err := os.WriteFile(path, []byte(`{
  "rules": [
    {
      "tag": "cyberpunk",
      "expr": "Metrics[\"spectral_flux\"] > 0.1 && Attributes[\"mode\"] == \"minor\"",
      "confidence": 0.9
    }
  ]
}`), 0o644)
	if err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	tagger, err := tags.NewRuleTaggerFromFile(path)
	if err != nil {
		t.Fatalf("new rule tagger from file: %v", err)
	}

	got, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{
		Metrics:    map[string]float64{"spectral_flux": 0.2},
		Attributes: map[string]string{"mode": "minor"},
	})
	if err != nil {
		t.Fatalf("rule tags: %v", err)
	}

	assertTagNames(t, got, "cyberpunk")
}

func TestRuleTaggerRejectsInvalidRuleConfig(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "rules.json")
	err := os.WriteFile(path, []byte(`[{"tag":"bad","expr":"Metrics[\"x\"] >"}]`), 0o644)
	if err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	if _, err := tags.NewRuleTaggerFromFile(path); err == nil {
		t.Fatal("expected invalid expr error")
	}
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
