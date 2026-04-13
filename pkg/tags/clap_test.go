package tags

import (
	"testing"
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
	// Verify the default label list is non-empty and contains expected entries.
	labels := defaultCLAPLabels
	if len(labels) == 0 {
		t.Error("defaultCLAPLabels should not be empty")
	}
	found := false
	for _, l := range labels {
		if l == "ambient" {
			found = true
			break
		}
	}
	if !found {
		t.Error("defaultCLAPLabels should contain 'ambient'")
	}
}
