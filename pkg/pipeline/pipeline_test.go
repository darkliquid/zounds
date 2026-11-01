package pipeline_test

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func openTestRepo(t *testing.T) *db.Repository {
	t.Helper()
	database, err := db.Open(context.Background(), db.Options{Path: filepath.Join(t.TempDir(), "zounds.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return db.NewRepository(database)
}

func indexWAV(t *testing.T, repo *db.Repository, root, name string, frequencies ...int) core.Sample {
	t.Helper()
	path := filepath.Join(root, name)
	writeBenchmarkWAV(t, path, benchmarkAudioBuffer(22050, frequencies...))

	ctx := context.Background()
	sampleID, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   root,
		Path:         path,
		RelativePath: name,
		FileName:     name,
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1024,
	})
	if err != nil {
		t.Fatalf("upsert sample %s: %v", path, err)
	}
	sample, err := repo.FindSampleByID(ctx, sampleID)
	if err != nil {
		t.Fatalf("find sample %s: %v", path, err)
	}
	return sample
}

func TestAnalyzeSamplesPersistsVectorAndRichAnalysis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	sample := indexWAV(t, repo, t.TempDir(), "tone.wav", 440)

	summary, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{sample}, pipeline.AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeSamples: %v", err)
	}
	if summary.Processed != 1 || summary.Skipped != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	vectors, err := repo.ListFeatureVectorsForSample(ctx, sample.ID)
	if err != nil {
		t.Fatalf("list feature vectors: %v", err)
	}
	if len(vectors) != 1 || vectors[0].Namespace != "analysis" {
		t.Fatalf("expected 1 analysis feature vector, got %+v", vectors)
	}

	rich, err := repo.GetSampleAnalysis(ctx, sample.ID)
	if err != nil {
		t.Fatalf("get sample analysis: %v", err)
	}
	if rich.DurationSeconds == nil {
		t.Fatalf("duration not persisted")
	}
	if got := *rich.DurationSeconds; got < 0.9 || got > 1.1 {
		t.Fatalf("unexpected duration: %v", got)
	}
	if rich.SampleRate == nil || *rich.SampleRate != 22050 {
		t.Fatalf("unexpected sample rate: %+v", rich.SampleRate)
	}
	if rich.Channels == nil || *rich.Channels != 2 {
		t.Fatalf("unexpected channels: %+v", rich.Channels)
	}
}

func TestAnalyzeSamplesSkipsUndecodableWithCallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()
	good := indexWAV(t, repo, root, "good.wav", 440)

	badPath := filepath.Join(root, "broken.aac")
	badID, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   root,
		Path:         badPath,
		RelativePath: "broken.aac",
		FileName:     "broken.aac",
		Extension:    "aac",
		Format:       core.FormatAAC,
		SizeBytes:    10,
	})
	if err != nil {
		t.Fatalf("upsert broken sample: %v", err)
	}
	bad, err := repo.FindSampleByID(ctx, badID)
	if err != nil {
		t.Fatalf("find broken sample: %v", err)
	}

	var skipped []string
	summary, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{good, bad}, pipeline.AnalyzeOptions{
		SkipUndecodable: true,
		Concurrency:     2,
		OnSkip: func(sample core.Sample, err error) {
			skipped = append(skipped, sample.Path)
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeSamples: %v", err)
	}
	if summary.Processed != 1 || summary.Skipped != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(skipped) != 1 || skipped[0] != badPath {
		t.Fatalf("unexpected skips: %v", skipped)
	}
}

func TestAnalyzeSamplesAbortsUndecodableWithoutSkipFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()
	badPath := filepath.Join(root, "broken.m4a")
	badID, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   root,
		Path:         badPath,
		RelativePath: "broken.m4a",
		FileName:     "broken.m4a",
		Extension:    "m4a",
		Format:       core.FormatAAC,
		SizeBytes:    10,
	})
	if err != nil {
		t.Fatalf("upsert broken sample: %v", err)
	}
	bad, err := repo.FindSampleByID(ctx, badID)
	if err != nil {
		t.Fatalf("find broken sample: %v", err)
	}

	if _, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{bad}, pipeline.AnalyzeOptions{}); err == nil {
		t.Fatal("expected error for undecodable sample without SkipUndecodable")
	}
}

func TestAutoTagPersistsTagsWithProvenance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()
	sample := indexWAV(t, repo, root, "808-boom.wav", 60)

	rulePath := filepath.Join(t.TempDir(), "rules.json")
	rules := `{
  "rules": [
    {
      "tag": "instrument:sub_bass",
      "expr": "Metrics[\"frequency_hz\"] < 120 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.9
    }
  ]
}`
	if err := os.WriteFile(rulePath, []byte(rules), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	var logOut bytes.Buffer
	summary, err := pipeline.AutoTag(ctx, repo, []core.Sample{sample}, pipeline.AutoTagOptions{
		RuleFile: rulePath,
		Logger:   log.New(&logOut, "", 0),
	})
	if err != nil {
		t.Fatalf("AutoTag: %v", err)
	}
	if summary.Processed != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	tags, err := repo.ListTagsForSample(ctx, sample.ID)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) == 0 {
		t.Fatalf("expected auto-generated tags for %s", sample.Path)
	}
	for _, tag := range tags {
		if tag.Source != "rules" {
			t.Fatalf("tag %q missing provenance source, got %q", tag.Name, tag.Source)
		}
	}
}

func TestSampleAnalysisFromResultMapsKnownFields(t *testing.T) {
	t.Parallel()

	result := core.AnalysisResult{
		SampleID: 42,
		Metrics: map[string]float64{
			"duration_seconds": 1.5,
			"sample_rate":      48000,
			"channels":         1,
			"tempo_bpm":        96.2,
			"key_confidence":   0.8,
			"frequency_hz":     220.0,
			"midi_note":        57.0,
			"loop_confidence":  0.9,
			"integrated_lufs":  -14.0,
		},
		Attributes: map[string]string{
			"tonic":      "A",
			"mode":       "minor",
			"loop_class": "loop",
		},
	}

	mapped := pipeline.SampleAnalysisFromResult(result, time.Now())
	if mapped.DurationSeconds == nil || *mapped.DurationSeconds != 1.5 {
		t.Fatalf("duration not mapped: %+v", mapped.DurationSeconds)
	}
	if mapped.SampleRate == nil || *mapped.SampleRate != 48000 {
		t.Fatalf("sample rate not mapped: %+v", mapped.SampleRate)
	}
	if mapped.BPM == nil || *mapped.BPM != 96.2 {
		t.Fatalf("bpm not mapped: %+v", mapped.BPM)
	}
	if mapped.KeyTonic == nil || *mapped.KeyTonic != "A" {
		t.Fatalf("key tonic not mapped: %+v", mapped.KeyTonic)
	}
	if mapped.LoopClass == nil || *mapped.LoopClass != "loop" {
		t.Fatalf("loop class not mapped: %+v", mapped.LoopClass)
	}
	if mapped.LUFSIntegrated == nil || *mapped.LUFSIntegrated != -14.0 {
		t.Fatalf("lufs not mapped: %+v", mapped.LUFSIntegrated)
	}
	if mapped.Chord != nil {
		t.Fatalf("chord should be nil when absent, got %+v", mapped.Chord)
	}
	if mapped.Metrics["tempo_bpm"] != 96.2 || mapped.Attributes["tonic"] != "A" {
		t.Fatalf("raw metrics/attributes must be preserved: %+v", mapped)
	}
}

func TestFindSimilarRanksNearestNeighbours(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()

	reference := indexWAV(t, repo, root, "kick.wav", 50, 100)
	near := indexWAV(t, repo, root, "kick-copy.wav", 55, 110)
	far := indexWAV(t, repo, root, "pad.wav", 440, 880)

	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{SampleID: reference.ID, Namespace: "analysis", Version: "1", Values: []float64{1, 0}}); err != nil {
		t.Fatalf("insert reference vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{SampleID: near.ID, Namespace: "analysis", Version: "1", Values: []float64{0.9, 0.1}}); err != nil {
		t.Fatalf("insert near vector: %v", err)
	}
	if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{SampleID: far.ID, Namespace: "analysis", Version: "1", Values: []float64{0, 1}}); err != nil {
		t.Fatalf("insert far vector: %v", err)
	}

	matches, err := pipeline.FindSimilar(ctx, repo, []string{"kick.wav"}, pipeline.SimilarOptions{})
	if err != nil {
		t.Fatalf("FindSimilar: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Sample.ID != near.ID {
		t.Fatalf("expected nearest neighbour %q first, got %q", near.Path, matches[0].Sample.Path)
	}
	if matches[1].Sample.ID != far.ID {
		t.Fatalf("expected far sample %q second, got %q", far.Path, matches[1].Sample.Path)
	}

	limited, err := pipeline.FindSimilar(ctx, repo, []string{"kick.wav"}, pipeline.SimilarOptions{Limit: 1})
	if err != nil {
		t.Fatalf("FindSimilar with limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limit to cap matches, got %d", len(limited))
	}
}

func TestClusterPersistsKMeansResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()

	for i, point := range [][]float64{{0, 0}, {0.1, 0}, {0, 0.1}, {5, 5}, {5.1, 5}, {5, 5.1}} {
		sample := indexWAV(t, repo, root, sampleName(i)+".wav", 100+i*50)
		if _, err := repo.InsertFeatureVector(ctx, core.FeatureVector{SampleID: sample.ID, Namespace: "analysis", Version: "1", Values: point}); err != nil {
			t.Fatalf("insert vector %d: %v", i, err)
		}
	}

	result, err := pipeline.Cluster(ctx, repo, pipeline.ClusterOptions{K: 2})
	if err != nil {
		t.Fatalf("Cluster: %v", err)
	}
	if len(result.Clusters) == 0 {
		t.Fatal("expected at least one cluster")
	}

	clusters, err := repo.ListClustersByMethod(ctx, "kmeans")
	if err != nil {
		t.Fatalf("list persisted clusters: %v", err)
	}
	if len(clusters) != len(result.Clusters) {
		t.Fatalf("expected %d persisted clusters, got %d", len(result.Clusters), len(clusters))
	}

	dry, err := pipeline.Cluster(ctx, repo, pipeline.ClusterOptions{K: 2, DryRun: true})
	if err != nil {
		t.Fatalf("Cluster dry run: %v", err)
	}
	if len(dry.Clusters) == 0 {
		t.Fatal("expected dry run to return clusters without persisting")
	}
}

func sampleName(i int) string {
	return string(rune('a' + i))
}

func TestAnalyzeSamplesSkipsUnchangedSamples(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()
	path := filepath.Join(root, "tone.wav")
	writeBenchmarkWAV(t, path, benchmarkAudioBuffer(22050, 440))

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	sampleID, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   root,
		Path:         path,
		RelativePath: "tone.wav",
		FileName:     "tone.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1024,
		ModifiedAt:   stat.ModTime(),
	})
	if err != nil {
		t.Fatalf("upsert sample: %v", err)
	}
	sample, err := repo.FindSampleByID(ctx, sampleID)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}

	first, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{sample}, pipeline.AnalyzeOptions{SkipUnchanged: true})
	if err != nil {
		t.Fatalf("first analyze: %v", err)
	}
	if first.Processed != 1 || first.Unchanged != 0 {
		t.Fatalf("first run should analyze: %+v", first)
	}

	second, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{sample}, pipeline.AnalyzeOptions{SkipUnchanged: true})
	if err != nil {
		t.Fatalf("second analyze: %v", err)
	}
	if second.Processed != 0 || second.Unchanged != 1 {
		t.Fatalf("second run should skip unchanged sample: %+v", second)
	}

	future := stat.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("touch fixture mtime: %v", err)
	}
	sample.ModifiedAt = future

	third, err := pipeline.AnalyzeSamples(ctx, repo, []core.Sample{sample}, pipeline.AnalyzeOptions{SkipUnchanged: true})
	if err != nil {
		t.Fatalf("third analyze: %v", err)
	}
	if third.Processed != 1 || third.Unchanged != 0 {
		t.Fatalf("modified sample must be re-analyzed: %+v", third)
	}
}

func TestAutoTagRunsConcurrently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)
	root := t.TempDir()

	rulePath := filepath.Join(t.TempDir(), "rules.json")
	rules := `{
  "rules": [
    {"tag": "deep", "expr": "Metrics[\"frequency_hz\"] < 120 && Metrics[\"confidence\"] > 0.5", "confidence": 0.9}
  ]
}`
	if err := os.WriteFile(rulePath, []byte(rules), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	var samples []core.Sample
	for i := 0; i < 4; i++ {
		samples = append(samples, indexWAV(t, repo, root, sampleName(i)+".wav", 60))
	}

	summary, err := pipeline.AutoTag(ctx, repo, samples, pipeline.AutoTagOptions{
		RuleFile:    rulePath,
		Concurrency: 4,
	})
	if err != nil {
		t.Fatalf("AutoTag: %v", err)
	}
	if summary.Processed != 4 {
		t.Fatalf("expected 4 processed, got %+v", summary)
	}

	for _, sample := range samples {
		tags, err := repo.ListTagsForSample(ctx, sample.ID)
		if err != nil {
			t.Fatalf("list tags for %s: %v", sample.Path, err)
		}
		if len(tags) != 1 || tags[0].NormalizedName != "deep" {
			t.Fatalf("expected rule tag on %s, got %+v", sample.Path, tags)
		}
	}
}
