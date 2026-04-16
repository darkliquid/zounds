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

func TestPathTaggerSkipsGenericAndPurelyNumericTokens(t *testing.T) {
	t.Parallel()

	tagger := tags.NewPathTagger()
	got, err := tagger.Tags(context.Background(), core.Sample{
		RelativePath: "Library/Samples/FX Pack/Kick_001.wav",
	}, core.AnalysisResult{})
	if err != nil {
		t.Fatalf("extract path tags: %v", err)
	}

	if len(got) != 1 || got[0].NormalizedName != "kick" {
		t.Fatalf("unexpected filtered tags %#v", got)
	}
}

func TestPathTaggerDecodesEscapedSegmentsAndPluses(t *testing.T) {
	t.Parallel()

	tagger := tags.NewPathTagger()
	got, err := tagger.Tags(context.Background(), core.Sample{
		RelativePath: "Black+Octopus+Sound/Didgeridoo%20Loops/BFD_Cm_88BPM.wav",
	}, core.AnalysisResult{})
	if err != nil {
		t.Fatalf("extract path tags: %v", err)
	}

	expected := map[string]struct{}{
		"black":      {},
		"octopus":    {},
		"didgeridoo": {},
		"bfd":        {},
		"cm":         {},
		"88bpm":      {},
	}

	if len(got) != len(expected) {
		t.Fatalf("unexpected tag count: got=%d want=%d (%v)", len(got), len(expected), got)
	}

	for _, tag := range got {
		if _, ok := expected[tag.NormalizedName]; !ok {
			t.Fatalf("unexpected tag %q", tag.NormalizedName)
		}
	}
}
