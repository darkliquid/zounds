package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
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
