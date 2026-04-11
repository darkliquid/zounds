package tags_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestLocalModelTaggerPredictsNearestTags(t *testing.T) {
	t.Parallel()

	tagger := tags.NewLocalModelTagger([]tags.TrainingExample{
		{
			Vector: core.FeatureVector{Values: []float64{0.1, 0.2, 0.15}},
			Tags: []core.Tag{
				{Name: "dark", NormalizedName: "dark"},
				{Name: "pad", NormalizedName: "pad"},
			},
		},
		{
			Vector: core.FeatureVector{Values: []float64{0.9, 0.8, 0.85}},
			Tags: []core.Tag{
				{Name: "bright", NormalizedName: "bright"},
				{Name: "stab", NormalizedName: "stab"},
			},
		},
	}, 1, 0.4, 3)

	predicted, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{
		FeatureVectors: []core.FeatureVector{{Values: []float64{0.12, 0.18, 0.14}}},
	})
	if err != nil {
		t.Fatalf("Tags returned error: %v", err)
	}
	if len(predicted) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(predicted))
	}
	if predicted[0].NormalizedName != "dark" {
		t.Fatalf("expected first tag to be dark, got %q", predicted[0].NormalizedName)
	}
	if predicted[0].Source != "local" {
		t.Fatalf("expected local source, got %q", predicted[0].Source)
	}
}

func TestLocalModelTaggerRejectsMissingVectors(t *testing.T) {
	t.Parallel()

	tagger := tags.NewLocalModelTagger([]tags.TrainingExample{
		{
			Vector: core.FeatureVector{Values: []float64{0.1, 0.2}},
			Tags:   []core.Tag{{Name: "dark", NormalizedName: "dark"}},
		},
	}, 1, 0.4, 3)

	if _, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{}); err == nil {
		t.Fatal("expected error for missing feature vectors")
	}
}
