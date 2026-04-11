package rename_test

import (
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/rename"
)

func TestOrganizeByTagsBuildsStableHierarchy(t *testing.T) {
	t.Parallel()

	plan := rename.OrganizeByTags("/library", core.Sample{
		FileName: "impact.wav",
	}, []core.Tag{
		{Name: "Dark", NormalizedName: "dark"},
		{Name: "Drums", NormalizedName: "drums"},
		{Name: "Dark", NormalizedName: "dark"},
	}, 2)

	expected := filepath.Clean("/library/dark/drums/impact.wav")
	if plan.TargetPath != expected {
		t.Fatalf("expected %q, got %q", expected, plan.TargetPath)
	}
}

func TestOrganizeByClusterUsesClusterLabel(t *testing.T) {
	t.Parallel()

	plan := rename.OrganizeByCluster("/clusters", core.Sample{
		FileName: "tone.wav",
	}, core.Cluster{Label: "Cluster 7"})

	expected := filepath.Clean("/clusters/cluster-7/tone.wav")
	if plan.TargetPath != expected {
		t.Fatalf("expected %q, got %q", expected, plan.TargetPath)
	}
}
