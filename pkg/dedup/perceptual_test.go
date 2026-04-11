package dedup_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/dedup"
)

func TestPerceptualFinderGroupsNearDuplicates(t *testing.T) {
	t.Parallel()

	finder := dedup.NewPerceptualFinder(2)
	groups, err := finder.Find([]dedup.PerceptualHash{
		{Sample: core.Sample{Path: "a.wav"}, Hash: "f0"},
		{Sample: core.Sample{Path: "b.wav"}, Hash: "f1"},
		{Sample: core.Sample{Path: "c.wav"}, Hash: "0f"},
	})
	if err != nil {
		t.Fatalf("find perceptual duplicates: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Reference.Path != "a.wav" {
		t.Fatalf("unexpected reference sample: %+v", groups[0].Reference)
	}
	if len(groups[0].Matches) != 1 || groups[0].Matches[0].Sample.Path != "b.wav" {
		t.Fatalf("unexpected group matches: %+v", groups[0].Matches)
	}
}

func TestPlanPerceptualCullRespectsKeepStrategy(t *testing.T) {
	t.Parallel()

	actions := dedup.PlanPerceptualCull([]dedup.PerceptualGroup{
		{
			Reference: core.Sample{Path: "b.wav"},
			Hash:      "f0",
			Matches: []dedup.PerceptualMatch{
				{Sample: core.Sample{Path: "a.wav"}},
				{Sample: core.Sample{Path: "c.wav"}},
			},
		},
	}, dedup.KeepFirstPath)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Keep.Path != "a.wav" {
		t.Fatalf("expected first path to be kept, got %+v", actions[0].Keep)
	}
	if len(actions[0].Remove) != 2 {
		t.Fatalf("expected 2 removals, got %+v", actions[0].Remove)
	}
}
