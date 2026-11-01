package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestClusterCommandPersistsKMeansClusters(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cluster.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/a.wav",
		RelativePath: "a.wav",
		FileName:     "a.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 0})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/b.wav",
		RelativePath: "b.wav",
		FileName:     "b.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.1, 0.1})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/c.wav",
		RelativePath: "c.wav",
		FileName:     "c.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{9.9, 10})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/d.wav",
		RelativePath: "d.wav",
		FileName:     "d.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10, 10.2})

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "cluster", "--method", "kmeans", "--k", "2"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute cluster: %v", err)
	}

	clusters, err := repo.ListClustersByMethod(ctx, "kmeans")
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 persisted clusters, got %d", len(clusters))
	}
	for _, item := range clusters {
		members, err := repo.ListClusterMembers(ctx, item.ID)
		if err != nil {
			t.Fatalf("list cluster members: %v", err)
		}
		if len(members) != 2 {
			t.Fatalf("expected 2 members in cluster %d, got %d", item.ID, len(members))
		}
	}
}

func TestClusterCommandDryRunPrintsSummary(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cluster-dry.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/a.wav",
		RelativePath: "a.wav",
		FileName:     "a.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 0})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/b.wav",
		RelativePath: "b.wav",
		FileName:     "b.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10, 10})

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "--dry-run", "cluster", "--k", "2"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute cluster dry-run: %v", err)
	}

	clusters, err := repo.ListClustersByMethod(ctx, "kmeans")
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 0 {
		t.Fatalf("expected no persisted clusters in dry-run, got %d", len(clusters))
	}
}

func TestClusterCommandMetricSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "cluster-metrics.db")

	f1 := filepath.Join(root, "f1.wav")
	f2 := filepath.Join(root, "f2.wav")
	f3 := filepath.Join(root, "f3.wav")

	writeSineWAVFixture(t, f1, 440, 1.0)
	writeSineWAVFixture(t, f2, 442, 1.0)
	writeSineWAVFixture(t, f3, 1760, 1.0)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("scan: %v", err)
	}

	analyze := commands.NewRootCommand()
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	for _, m := range []string{"acoustic", "timbre", "harmonic", "rhythm", "perceptual", "hybrid"} {
		cmd := commands.NewRootCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--db", dbPath, "cluster", "--metric", m, "--k", "2"})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("metric %s: execute cluster: %v", m, err)
		}
		if !strings.Contains(out.String(), "cluster-") {
			t.Fatalf("metric %s: expected cluster labels in output:\n%s", m, out.String())
		}
	}
}

func TestClusterCommandDBSCAN(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cluster-dbscan.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/a.wav",
		RelativePath: "a.wav",
		FileName:     "a.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 0})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/b.wav",
		RelativePath: "b.wav",
		FileName:     "b.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.1, 0.1})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/c.wav",
		RelativePath: "c.wav",
		FileName:     "c.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10, 10})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/d.wav",
		RelativePath: "d.wav",
		FileName:     "d.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10.1, 10.1})

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "cluster", "--method", "dbscan", "--eps", "0.5", "--min-points", "2"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute dbscan cluster: %v", err)
	}

	clusters, err := repo.ListClustersByMethod(ctx, "dbscan")
	if err != nil {
		t.Fatalf("list dbscan clusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 persisted dbscan clusters, got %d", len(clusters))
	}
	if !strings.Contains(out.String(), "cluster-1") {
		t.Fatalf("expected summary output to contain cluster-1, got:\n%s", out.String())
	}
}

func TestClusterCommandHDBSCAN(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cluster-hdbscan.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	// Group 1 (3 points)
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/a.wav",
		RelativePath: "a.wav",
		FileName:     "a.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 0})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/b.wav",
		RelativePath: "b.wav",
		FileName:     "b.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.05, 0.05})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/c.wav",
		RelativePath: "c.wav",
		FileName:     "c.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{-0.05, -0.05})

	// Group 2 (3 points)
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/d.wav",
		RelativePath: "d.wav",
		FileName:     "d.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10, 10})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/e.wav",
		RelativePath: "e.wav",
		FileName:     "e.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10.05, 10.05})
	insertSampleWithVector(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/f.wav",
		RelativePath: "f.wav",
		FileName:     "f.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{9.95, 9.95})

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "cluster", "--method", "hdbscan", "--min-cluster-size", "3"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute hdbscan cluster: %v", err)
	}

	clusters, err := repo.ListClustersByMethod(ctx, "hdbscan")
	if err != nil {
		t.Fatalf("list hdbscan clusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 persisted hdbscan clusters, got %d", len(clusters))
	}
}

func TestClusterCommandAutoLabelsFromDominantTags(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "cluster-autolabel.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	id1 := insertSampleWithVectorID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/kick1.wav",
		RelativePath: "kick1.wav",
		FileName:     "kick1.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 0})
	id2 := insertSampleWithVectorID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/kick2.wav",
		RelativePath: "kick2.wav",
		FileName:     "kick2.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.1, 0.1})

	id3 := insertSampleWithVectorID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/snare1.wav",
		RelativePath: "snare1.wav",
		FileName:     "snare1.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10, 10})
	id4 := insertSampleWithVectorID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/snare2.wav",
		RelativePath: "snare2.wav",
		FileName:     "snare2.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{10.1, 10.1})

	// Tag kick samples with instrument:kick
	kickTag := core.Tag{Name: "instrument:kick", NormalizedName: "instrument:kick", Confidence: 0.95, Source: "test"}
	tagID1, err := repo.EnsureTag(ctx, kickTag)
	if err != nil {
		t.Fatalf("ensure kick tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, id1, tagID1, kickTag.Source, kickTag.Confidence); err != nil {
		t.Fatalf("add tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, id2, tagID1, kickTag.Source, kickTag.Confidence); err != nil {
		t.Fatalf("add tag: %v", err)
	}

	// Tag snare samples with instrument:snare
	snareTag := core.Tag{Name: "instrument:snare", NormalizedName: "instrument:snare", Confidence: 0.90, Source: "test"}
	tagID2, err := repo.EnsureTag(ctx, snareTag)
	if err != nil {
		t.Fatalf("ensure snare tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, id3, tagID2, snareTag.Source, snareTag.Confidence); err != nil {
		t.Fatalf("add tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, id4, tagID2, snareTag.Source, snareTag.Confidence); err != nil {
		t.Fatalf("add tag: %v", err)
	}

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "cluster", "--method", "kmeans", "--k", "2"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute cluster with autolabel: %v", err)
	}

	clusters, err := repo.ListClustersByMethod(ctx, "kmeans")
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}

	outStr := out.String()
	if !strings.Contains(outStr, "instrument:kick") {
		t.Fatalf("expected output to contain instrument:kick, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "instrument:snare") {
		t.Fatalf("expected output to contain instrument:snare, got:\n%s", outStr)
	}

	// Also check persisted labels
	labels := []string{clusters[0].Label, clusters[1].Label}
	foundKick := false
	foundSnare := false
	for _, l := range labels {
		if strings.Contains(l, "instrument:kick") {
			foundKick = true
		}
		if strings.Contains(l, "instrument:snare") {
			foundSnare = true
		}
	}
	if !foundKick || !foundSnare {
		t.Fatalf("expected persisted clusters to be auto-labeled with kick and snare, got labels: %+v", labels)
	}
}

func insertSampleWithVectorID(t *testing.T, ctx context.Context, repo interface {
	UpsertSample(context.Context, core.Sample) (int64, error)
	ReplaceFeatureVector(context.Context, core.FeatureVector) (int64, error)
}, sample core.Sample, values []float64) int64 {
	t.Helper()

	sampleID, err := repo.UpsertSample(ctx, sample)
	if err != nil {
		t.Fatalf("upsert sample: %v", err)
	}
	if _, err := repo.ReplaceFeatureVector(ctx, core.FeatureVector{
		SampleID:   sampleID,
		Namespace:  "analysis",
		Version:    "test",
		Values:     values,
		Dimensions: len(values),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("replace feature vector: %v", err)
	}
	return sampleID
}

func insertSampleWithVector(t *testing.T, ctx context.Context, repo interface {
	UpsertSample(context.Context, core.Sample) (int64, error)
	ReplaceFeatureVector(context.Context, core.FeatureVector) (int64, error)
}, sample core.Sample, values []float64) {
	t.Helper()

	sampleID, err := repo.UpsertSample(ctx, sample)
	if err != nil {
		t.Fatalf("upsert sample: %v", err)
	}
	if _, err := repo.ReplaceFeatureVector(ctx, core.FeatureVector{
		SampleID:   sampleID,
		Namespace:  "analysis",
		Version:    "test",
		Values:     values,
		Dimensions: len(values),
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("replace feature vector: %v", err)
	}
}
