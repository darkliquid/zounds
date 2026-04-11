package tags

import (
	"context"
	"strings"

	"github.com/darkliquid/zounds/pkg/core"
)

const metadataTaggerVersion = "0.1.0"

type MetadataTagger struct{}

func NewMetadataTagger() MetadataTagger {
	return MetadataTagger{}
}

func (MetadataTagger) Name() string {
	return "metadata"
}

func (MetadataTagger) Version() string {
	return metadataTaggerVersion
}

func (MetadataTagger) Tags(_ context.Context, _ core.Sample, result core.AnalysisResult) ([]core.Tag, error) {
	seen := map[string]struct{}{}
	var tags []core.Tag

	for _, key := range []string{
		"embedded.genre",
		"embedded.artist",
		"embedded.album_artist",
		"embedded.composer",
		"embedded.album",
		"embedded.title",
		"embedded.comment",
	} {
		value := result.Attributes[key]
		if strings.TrimSpace(value) == "" {
			continue
		}

		for _, part := range splitMetadataValues(value) {
			normalized := core.NormalizeTagName(part)
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
				Source:         "metadata",
				Confidence:     1.0,
			})
		}
	}

	return tags, nil
}

func splitMetadataValues(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ';', '/', '|':
			return true
		default:
			return false
		}
	})
}
