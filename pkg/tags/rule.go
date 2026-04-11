package tags

import (
	"context"

	"github.com/darkliquid/zounds/pkg/core"
)

const ruleTaggerVersion = "0.1.0"

type RuleTagger struct{}

func NewRuleTagger() RuleTagger {
	return RuleTagger{}
}

func (RuleTagger) Name() string {
	return "rules"
}

func (RuleTagger) Version() string {
	return ruleTaggerVersion
}

func (RuleTagger) Tags(_ context.Context, _ core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	var tags []core.Tag

	centroid := result.Metrics["spectral_centroid_hz"]
	dominant := result.Metrics["dominant_frequency_hz"]
	flatness := result.Metrics["spectral_flatness"]
	flux := result.Metrics["spectral_flux"]
	zcr := result.Metrics["zero_crossing_rate"]
	attack := result.Metrics["attack_sharpness"]
	sustain := result.Metrics["sustain_ratio"]
	transientRate := result.Metrics["transient_rate"]
	frequency := result.Metrics["frequency_hz"]
	confidence := result.Metrics["confidence"]

	if centroid > 0 && centroid < 1500 && dominant > 0 && dominant < 400 && flatness < 0.3 {
		tags = append(tags, newRuleTag("dark", 0.72))
	}
	if dominant > 1000 && flatness < 0.2 && sustain > 0.35 && confidence > 0.35 {
		tags = append(tags, newRuleTag("bell", 0.68))
	}
	if flux > 0.12 && transientRate > 20 && zcr > 0.08 {
		tags = append(tags, newRuleTag("glitch", 0.7))
	}
	if sustain > 0.65 && transientRate < 40 && attack < 0.35 && flatness < 0.35 {
		tags = append(tags, newRuleTag("pad", 0.66))
	}
	if attack > 0.55 && sustain < 0.45 {
		tags = append(tags, newRuleTag("stab", 0.62))
	}
	if frequency > 0 && frequency < 120 && confidence > 0.5 {
		tags = append(tags, newRuleTag("sub", 0.75))
	}
	if frequency >= 80 && frequency <= 400 && sustain > 0.45 && flux > 0.03 && flux < 0.2 && flatness > 0.05 && flatness < 0.4 && confidence > 0.4 {
		tags = append(tags, newRuleTag("hoover", 0.58))
	}

	return uniqueTags(tags), nil
}

func newRuleTag(name string, confidence float64) core.Tag {
	normalized := core.NormalizeTagName(name)
	return core.Tag{
		Name:           normalized,
		NormalizedName: normalized,
		Source:         "rules",
		Confidence:     confidence,
	}
}

func uniqueTags(tags []core.Tag) []core.Tag {
	seen := make(map[string]struct{}, len(tags))
	out := make([]core.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag.NormalizedName == "" {
			continue
		}
		if _, ok := seen[tag.NormalizedName]; ok {
			continue
		}
		seen[tag.NormalizedName] = struct{}{}
		out = append(out, tag)
	}
	return out
}
