package commands_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestEndToEndWorkflowScanAnalyzeTagAndCluster(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "workflow.db")
	writeSineWAVFixture(t, filepath.Join(root, "Bass", "sub.wav"), 55, 1.0)
	writeSineWAVFixture(t, filepath.Join(root, "Leads", "lead.wav"), 880, 1.0)

	runCommand(t, dbPath, "scan", root)
	runCommand(t, dbPath, "analyze", "--all")
	runCommand(t, dbPath, "tag", "--auto")

	clusterOut := runCommand(t, dbPath, "cluster", "--k", "2")
	if !strings.Contains(clusterOut, "cluster-1") {
		t.Fatalf("unexpected cluster output: %q", clusterOut)
	}

	repo := openRepoForTest(t, dbPath)
	samples, err := repo.ListSamples(context.Background())
	if err != nil {
		t.Fatalf("ListSamples returned error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 indexed samples, got %d", len(samples))
	}

	for _, sample := range samples {
		vectors, err := repo.ListFeatureVectorsForSample(context.Background(), sample.ID)
		if err != nil {
			t.Fatalf("ListFeatureVectorsForSample returned error: %v", err)
		}
		if len(vectors) == 0 {
			t.Fatalf("expected feature vector for sample %d", sample.ID)
		}

		tags, err := repo.ListTagsForSample(context.Background(), sample.ID)
		if err != nil {
			t.Fatalf("ListTagsForSample returned error: %v", err)
		}
		if len(tags) == 0 {
			t.Fatalf("expected tags for sample %d", sample.ID)
		}
	}

	clusters, err := repo.ListClustersByMethod(context.Background(), "kmeans")
	if err != nil {
		t.Fatalf("ListClustersByMethod returned error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
}

func runCommand(t *testing.T, dbPath string, args ...string) string {
	t.Helper()

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"--db", dbPath}, args...))
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute %v: %v\noutput: %s", args, err, out.String())
	}
	return out.String()
}
