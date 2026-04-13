package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	"github.com/darkliquid/zounds/pkg/analysis"
)

func TestAnalyzeCommandPersistsFeatureVector(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "analyze.db")
	samplePath := filepath.Join(root, "tones", "a440.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	analyze := commands.NewRootCommand()
	var out bytes.Buffer
	analyze.SetOut(&out)
	analyze.SetErr(&out)
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute analyze: %v", err)
	}
	if !strings.Contains(out.String(), "analyzed 1 samples") {
		t.Fatalf("unexpected analyze output: %q", out.String())
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	vectors, err := repo.ListFeatureVectorsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list feature vectors: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected 1 feature vector, got %d", len(vectors))
	}
	if vectors[0].Namespace != "analysis" {
		t.Fatalf("unexpected vector namespace: %s", vectors[0].Namespace)
	}
	if vectors[0].Dimensions != len(analysis.FeatureNames()) {
		t.Fatalf("unexpected vector dimensions: got=%d want=%d", vectors[0].Dimensions, len(analysis.FeatureNames()))
	}
}

func TestAnalyzeCommandVerboseShowsPerSampleProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "analyze-verbose.db")
	samplePath := filepath.Join(root, "tones", "a440.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	analyze := commands.NewRootCommand()
	var out bytes.Buffer
	analyze.SetOut(&out)
	analyze.SetErr(&out)
	analyze.SetArgs([]string{"--verbose", "--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute analyze: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"verbose: analyzing sample " + samplePath,
		"verbose: persisting feature vector for " + samplePath,
		"analyzed 1 samples",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestInfoCommandOutputsAnalysisJSON(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "info.wav")
	writeWAVFixture(t, path)

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"info", path})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute info: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode info JSON: %v", err)
	}
	if payload["path"] != path {
		t.Fatalf("unexpected path: %v", payload["path"])
	}
	if _, ok := payload["feature_vector"]; !ok {
		t.Fatalf("missing feature_vector in payload")
	}
	analyzers, ok := payload["analyzers"].(map[string]any)
	if !ok {
		t.Fatalf("missing analyzers payload")
	}
	if _, ok := analyzers["key"]; !ok {
		t.Fatalf("expected key analyzer output")
	}
	if _, ok := analyzers["hpss"]; !ok {
		t.Fatalf("expected hpss analyzer output")
	}
}
