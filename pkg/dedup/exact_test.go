package dedup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/dedup"
)

func TestExactFinderFindsDuplicateGroups(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dupA := filepath.Join(root, "dup-a.wav")
	dupB := filepath.Join(root, "dup-b.wav")
	unique := filepath.Join(root, "unique.wav")

	writeFile(t, dupA, []byte("same"))
	writeFile(t, dupB, []byte("same"))
	writeFile(t, unique, []byte("different"))

	finder := dedup.NewExactFinder(2)
	groups, err := finder.Find(context.Background(), []core.Sample{
		sampleFromPath(t, dupA),
		sampleFromPath(t, dupB),
		sampleFromPath(t, unique),
	})
	if err != nil {
		t.Fatalf("find duplicates: %v", err)
	}

	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if len(groups[0].Samples) != 2 {
		t.Fatalf("expected 2 samples in duplicate group, got %d", len(groups[0].Samples))
	}
}

func TestPlanCullKeepsOldest(t *testing.T) {
	t.Parallel()

	oldest := core.Sample{Path: "/a.wav", ModifiedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	newest := core.Sample{Path: "/b.wav", ModifiedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	actions := dedup.PlanCull([]dedup.DuplicateGroup{{
		Hash:    "abc",
		Samples: []core.Sample{newest, oldest},
	}}, dedup.KeepOldest)

	if len(actions) != 1 {
		t.Fatalf("expected 1 cull action, got %d", len(actions))
	}
	if actions[0].Keep.Path != oldest.Path {
		t.Fatalf("expected oldest file to be kept, got %s", actions[0].Keep.Path)
	}
	if len(actions[0].Remove) != 1 || actions[0].Remove[0].Path != newest.Path {
		t.Fatalf("unexpected removal set: %+v", actions[0].Remove)
	}
}

func sampleFromPath(t *testing.T, path string) core.Sample {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	return core.Sample{
		Path:       path,
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
