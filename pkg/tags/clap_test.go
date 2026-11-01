package tags

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/clap"
)

func TestLocalCLAPTagger_Name(t *testing.T) {
	tagger := &LocalCLAPTagger{}
	if got := tagger.Name(); got != "clap" {
		t.Errorf("Name() = %q, want %q", got, "clap")
	}
}

func TestLocalCLAPTagger_Version(t *testing.T) {
	tagger := &LocalCLAPTagger{}
	if got := tagger.Version(); got != localCLAPTaggerVersion {
		t.Errorf("Version() = %q, want %q", got, localCLAPTaggerVersion)
	}
}

func TestNewLocalCLAPTagger_MissingModelDir(t *testing.T) {
	_, err := NewLocalCLAPTagger("/nonexistent/clap_model", "", nil, 0, 0)
	if err == nil {
		t.Error("expected an error when model directory does not exist")
	}
}

func TestNewLocalCLAPTagger_DefaultLabels(t *testing.T) {
	labels := defaultCLAPLabels
	if len(labels) == 0 {
		t.Error("defaultCLAPLabels should not be empty")
	}
	expected := map[string]struct{}{
		"instrument:kick":       {},
		"instrument:snare":      {},
		"articulation:oneshot":  {},
		"articulation:loop":     {},
		"character:dark":        {},
		"genre:techno":          {},
	}
	for _, label := range labels {
		delete(expected, label)
	}
	if len(expected) != 0 {
		t.Fatalf("defaultCLAPLabels missing expected faceted entries: %#v", expected)
	}
}

func TestLocalCLAPTaggerTagsForScoresTightensDefaultLabels(t *testing.T) {
	tagger := &LocalCLAPTagger{
		defaultsUsed: true,
		minScore:     defaultCLAPMinScore,
		maxPredicted: defaultCLAPMaxPredicted,
	}

	got := tagger.tagsForScores([]clap.LabelScore{
		{Label: "instrument:pad", Score: 0.44},
		{Label: "instrument:bass", Score: 0.40},
		{Label: "character:metallic", Score: 0.12},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 retained labels, got %#v", got)
	}
	if got[0].NormalizedName != "instrument:pad" || got[1].NormalizedName != "instrument:bass" {
		t.Fatalf("unexpected retained labels %#v", got)
	}
}

func TestTaxonomy_ParseAndFormat(t *testing.T) {
	facet, val, ok := ParseTag("Instrument:Sub_Bass")
	if !ok || facet != "instrument" || val != "sub bass" {
		t.Fatalf("unexpected ParseTag: facet=%q val=%q ok=%v", facet, val, ok)
	}

	facet, val, ok = ParseTag("unfaceted")
	if ok || facet != "" || val != "unfaceted" {
		t.Fatalf("unexpected unfaceted ParseTag: facet=%q val=%q ok=%v", facet, val, ok)
	}

	formatted := FormatTag("character", "Dark_Metallic")
	if formatted != "character:dark metallic" {
		t.Fatalf("unexpected FormatTag: %q", formatted)
	}

	for _, sf := range StandardFacets {
		if !IsStandardFacet(sf) {
			t.Errorf("expected %q to be standard facet", sf)
		}
	}
	if IsStandardFacet("unknown_facet") {
		t.Errorf("did not expect unknown_facet to be standard")
	}
}

func TestTaxonomy_PromptEnsembleForTag(t *testing.T) {
	cases := []struct {
		tag      string
		minCount int
	}{
		{"instrument:kick", 4},
		{"role:lead", 4},
		{"character:metallic", 4},
		{"articulation:oneshot", 4},
		{"articulation:loop", 4},
		{"genre:techno", 4},
		{"freeform_sound", 3},
	}

	for _, tc := range cases {
		prompts := PromptEnsembleForTag(tc.tag)
		if len(prompts) < tc.minCount {
			t.Errorf("PromptEnsembleForTag(%q) produced %d prompts, want >= %d", tc.tag, len(prompts), tc.minCount)
		}
	}
}
