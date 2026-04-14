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
		"kick":       {},
		"snare":      {},
		"impact":     {},
		"vocal chop": {},
	}
	for _, label := range labels {
		delete(expected, label)
	}
	if len(expected) != 0 {
		t.Fatalf("defaultCLAPLabels missing expected entries: %#v", expected)
	}
}

func TestLocalCLAPTaggerTagsForScoresTightensDefaultLabels(t *testing.T) {
	tagger := &LocalCLAPTagger{
		defaultsUsed: true,
		minScore:     defaultCLAPMinScore,
		maxPredicted: defaultCLAPMaxPredicted,
	}

	got := tagger.tagsForScores([]clap.LabelScore{
		{Label: "pad", Score: 0.44},
		{Label: "bass", Score: 0.40},
		{Label: "impact", Score: 0.31},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 retained labels, got %#v", got)
	}
	if got[0].NormalizedName != "pad" || got[1].NormalizedName != "bass" {
		t.Fatalf("unexpected retained labels %#v", got)
	}
}
