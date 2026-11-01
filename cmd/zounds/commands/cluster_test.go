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
