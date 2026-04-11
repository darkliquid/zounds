package rename

import (
	"path/filepath"
	"slices"

	"github.com/darkliquid/zounds/pkg/core"
)

type OrganizationPlan struct {
	Sample     core.Sample
	TargetPath string
}

func OrganizeByTags(baseDir string, sample core.Sample, tags []core.Tag, maxDepth int) OrganizationPlan {
	names := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		name := tag.NormalizedName
		if name == "" {
			name = core.NormalizeTagName(tag.Name)
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, slug(name))
	}

	slices.Sort(names)
	if maxDepth > 0 && len(names) > maxDepth {
		names = names[:maxDepth]
	}

	target := baseDir
	if len(names) > 0 {
		target = filepath.Join(append([]string{baseDir}, names...)...)
	}
	target = filepath.Join(target, sample.FileName)

	return OrganizationPlan{
		Sample:     sample,
		TargetPath: filepath.Clean(target),
	}
}

func OrganizeByCluster(baseDir string, sample core.Sample, cluster core.Cluster) OrganizationPlan {
	clusterDir := slug(cluster.Label)
	if clusterDir == "" {
		clusterDir = "unclustered"
	}

	return OrganizationPlan{
		Sample:     sample,
		TargetPath: filepath.Clean(filepath.Join(baseDir, clusterDir, sample.FileName)),
	}
}
