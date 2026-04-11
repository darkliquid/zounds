package tags_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestMetadataTaggerExtractsTagsFromEmbeddedAttributes(t *testing.T) {
	t.Parallel()

	tagger := tags.NewMetadataTagger()
	got, err := tagger.Tags(context.Background(), core.Sample{}, core.AnalysisResult{
		Attributes: map[string]string{
			"embedded.genre":   "Cyberpunk/Dark Ambient",
			"embedded.artist":  "Unit Test",
			"embedded.comment": "Neon; Rain",
		},
	})
	if err != nil {
		t.Fatalf("extract metadata tags: %v", err)
	}

	expected := map[string]struct{}{
		"cyberpunk":    {},
		"dark ambient": {},
		"unit test":    {},
		"neon":         {},
		"rain":         {},
	}

	if len(got) != len(expected) {
		t.Fatalf("unexpected tag count: got=%d want=%d (%v)", len(got), len(expected), got)
	}

	for _, tag := range got {
		if _, ok := expected[tag.NormalizedName]; !ok {
			t.Fatalf("unexpected metadata tag %q", tag.NormalizedName)
		}
		if tag.Source != "metadata" {
			t.Fatalf("unexpected metadata tag source: %s", tag.Source)
		}
	}
}
