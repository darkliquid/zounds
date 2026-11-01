package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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

func TestAnalyzeCommandWithCLAP(t *testing.T) {
	modelDir := filepath.Clean("../../../onnx")
	for _, required := range []string{
		filepath.Join(modelDir, "audio_model.onnx"),
		filepath.Join(modelDir, "text_model.onnx"),
		filepath.Join(modelDir, "tokenizer.json"),
	} {
		if _, err := os.Stat(required); err != nil {
			t.Skipf("skipping CLAP analyze test (%s not present)", required)
		}
	}

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "analyze-clap.db")
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
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all", "--clap-model-dir", modelDir})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Skipf("execute analyze with CLAP skipped: %v", err)
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
	if len(vectors) != 2 {
		t.Fatalf("expected 2 feature vectors (analysis + clap), got %d", len(vectors))
	}

	clapVec, err := repo.FindFeatureVector(context.Background(), sample.ID, "clap")
	if err != nil {
		t.Fatalf("find clap vector: %v", err)
	}
	if clapVec.Dimensions != 512 || len(clapVec.Values) != 512 {
		t.Fatalf("expected 512-D clap vector, got %d", clapVec.Dimensions)
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

func TestAnalyzeCommandAllSkipsUndecodableSamplesWithWarning(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "analyze-skip.db")
	validWav := filepath.Join(root, "valid.wav")
	unsupportedM4A := filepath.Join(root, "unsupported.m4a")
	corruptWav := filepath.Join(root, "corrupt.wav")

	writeWAVFixture(t, validWav)
	if err := os.WriteFile(unsupportedM4A, []byte("fake-m4a"), 0o644); err != nil {
		t.Fatalf("write m4a: %v", err)
	}
	if err := os.WriteFile(corruptWav, []byte("corrupt-riff"), 0o644); err != nil {
		t.Fatalf("write corrupt wav: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	analyze := commands.NewRootCommand()
	var stdout, stderr bytes.Buffer
	analyze.SetOut(&stdout)
	analyze.SetErr(&stderr)
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("expected analyze --all to succeed while skipping undecodables, got: %v", err)
	}

	if !strings.Contains(stdout.String(), "analyzed 1 samples") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	for _, skipped := range []string{unsupportedM4A, corruptWav} {
		if !strings.Contains(stderr.String(), skipped) {
			t.Fatalf("expected warning mentioning %q in stderr %q", skipped, stderr.String())
		}
	}
}
