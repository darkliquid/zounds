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

func TestPerceptualFinderPreventsTransitiveChaining(t *testing.T) {
	t.Parallel()

	// A: 0000000000000000
	// B: 0000000000000003 (differs from A by 2 bits)
	// C: 000000000000000f (differs from B by 2 bits, differs from A by 4 bits)
	// With MaxDistance = 2:
	// A and B can group, or B and C can group, but A and C MUST NOT be in the same group!
	finder := dedup.NewPerceptualFinder(2)
	groups, err := finder.Find([]dedup.PerceptualHash{
		{Sample: core.Sample{Path: "a.wav"}, Hash: "0000000000000000"},
		{Sample: core.Sample{Path: "b.wav"}, Hash: "0000000000000003"},
		{Sample: core.Sample{Path: "c.wav"}, Hash: "000000000000000f"},
	})
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	for _, g := range groups {
		paths := map[string]bool{g.Reference.Path: true}
		for _, m := range g.Matches {
			paths[m.Sample.Path] = true
		}
		if paths["a.wav"] && paths["c.wav"] {
			t.Fatalf("transitive chaining detected: a.wav and c.wav (distance 4) are in the same group with maxDistance 2")
		}
	}
}

func TestPlanPerceptualCullBestQuality(t *testing.T) {
	t.Parallel()

	actions := dedup.PlanPerceptualCull([]dedup.PerceptualGroup{
		{
			Reference: core.Sample{
				Path:   "track.mp3",
				Format: core.FormatMP3,
			},
			Hash: "0000000000000000",
			Matches: []dedup.PerceptualMatch{
				{
					Sample: core.Sample{
						Path:   "track.wav",
						Format: core.FormatWAV,
						Metadata: map[string]string{
							"sample_rate": "44100",
							"bit_depth":   "16",
						},
					},
				},
				{
					Sample: core.Sample{
						Path:   "track_hires.wav",
						Format: core.FormatWAV,
						Metadata: map[string]string{
							"sample_rate": "96000",
							"bit_depth":   "24",
						},
					},
				},
			},
		},
	}, dedup.KeepBestQuality)

	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	// Highest fidelity is track_hires.wav (WAV + 96kHz + 24bit)
	if actions[0].Keep.Path != "track_hires.wav" {
		t.Fatalf("expected track_hires.wav to be kept, got %q", actions[0].Keep.Path)
	}
	if len(actions[0].Remove) != 2 {
		t.Fatalf("expected 2 removals, got %d", len(actions[0].Remove))
	}
}
