package tags_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/tags"
)

func TestPathTaggerExtractsNormalizedUniqueTags(t *testing.T) {
	t.Parallel()

	tagger := tags.NewPathTagger()
	got, err := tagger.Tags(context.Background(), core.Sample{
		RelativePath: "Drums/808 Kicks/Dark-Glitch_Hit_01.wav",
	}, core.AnalysisResult{})
	if err != nil {
		t.Fatalf("extract path tags: %v", err)
	}

	expected := map[string]struct{}{
		"drums":  {},
		"808":    {},
		"kicks":  {},
		"dark":   {},
		"glitch": {},
		"hit":    {},
		"01":     {},
	}

	if len(got) != len(expected) {
		t.Fatalf("unexpected tag count: got=%d want=%d (%v)", len(got), len(expected), got)
	}

	for _, tag := range got {
		if _, ok := expected[tag.NormalizedName]; !ok {
			t.Fatalf("unexpected tag %q", tag.NormalizedName)
		}
		if tag.Source != "path" {
			t.Fatalf("unexpected tag source: %s", tag.Source)
		}
	}
}
