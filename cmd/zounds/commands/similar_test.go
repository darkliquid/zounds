package commands_test

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

func TestSimilarCommandListsBestMatchesByDescendingSimilarity(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "similar.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	referenceID := insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/ref.wav",
		RelativePath: "ref.wav",
		FileName:     "ref.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{1, 0})
	nearID := insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/near.wav",
		RelativePath: "near.wav",
		FileName:     "near.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.95, 0.05})
	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/mid.wav",
		RelativePath: "mid.wav",
		FileName:     "mid.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.7, 0.3})
	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/far.wav",
		RelativePath: "far.wav",
		FileName:     "far.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 1})

	output := runCommand(t, dbPath, "similar", "--threshold", "0.8", "--limit", "1", "ref.wav")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 output line, got %q", output)
	}
	if !strings.Contains(lines[0], "/tmp/ref.wav") || !strings.Contains(lines[0], "/tmp/near.wav") {
		t.Fatalf("unexpected output line: %q", lines[0])
	}

	idOutput := runCommand(t, dbPath, "similar", "--threshold", "0.8", "--limit", "1", strconv.FormatInt(referenceID, 10), strconv.FormatInt(nearID, 10))
	if !strings.Contains(idOutput, "/tmp/mid.wav") {
		t.Fatalf("expected id-based lookup to return mid sample, got %q", idOutput)
	}
}

func TestSimilarCommandUsesBestReferenceAndAscendingOrder(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "similar-multi.db")
	repo := openRepoForTest(t, dbPath)
	ctx := context.Background()

	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/left.wav",
		RelativePath: "left.wav",
		FileName:     "left.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{1, 0})
	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/right.wav",
		RelativePath: "right.wav",
		FileName:     "right.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0, 1})
	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/left-match.wav",
		RelativePath: "left-match.wav",
		FileName:     "left-match.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.8, 0.2})
	insertSampleWithVectorAndReturnID(t, ctx, repo, core.Sample{
		SourceRoot:   "/tmp",
		Path:         "/tmp/right-match.wav",
		RelativePath: "right-match.wav",
		FileName:     "right-match.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    1,
		ScannedAt:    time.Now().UTC(),
	}, []float64{0.1, 0.9})

	output := runCommand(t, dbPath, "similar", "--threshold", "0.95", "--order", "asc", "left.wav", "right.wav")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines, got %q", output)
	}
	if !strings.Contains(lines[0], "/tmp/left.wav") || !strings.Contains(lines[0], "/tmp/left-match.wav") {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if !strings.Contains(lines[1], "/tmp/right.wav") || !strings.Contains(lines[1], "/tmp/right-match.wav") {
		t.Fatalf("unexpected second line: %q", lines[1])
	}
}

func insertSampleWithVectorAndReturnID(t *testing.T, ctx context.Context, repo interface {
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
