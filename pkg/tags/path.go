package tags

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

const pathTaggerVersion = "0.1.0"

type PathTagger struct{}

func NewPathTagger() PathTagger {
	return PathTagger{}
}

func (PathTagger) Name() string {
	return "path"
}

func (PathTagger) Version() string {
	return pathTaggerVersion
}

func (PathTagger) Tags(_ context.Context, sample core.Sample, _ core.AnalysisResult) ([]core.Tag, error) {
	sourcePath := sample.RelativePath
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = sample.Path
	}

	sourcePath = filepath.ToSlash(sourcePath)
	segments := strings.Split(sourcePath, "/")
	seen := map[string]struct{}{}
	tags := make([]core.Tag, 0, len(segments))

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		segment = strings.TrimSuffix(segment, filepath.Ext(segment))
		for _, token := range tokenizePathSegment(segment) {
			normalized := core.NormalizeTagName(token)
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}

			seen[normalized] = struct{}{}
			tags = append(tags, core.Tag{
				Name:           normalized,
				NormalizedName: normalized,
				Source:         "path",
				Confidence:     1.0,
			})
		}
	}

	return tags, nil
}

func tokenizePathSegment(segment string) []string {
	segment = strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', '(', ')', '[', ']', '{', '}':
			return ' '
		default:
			return r
		}
	}, segment)

	return strings.Fields(segment)
}
