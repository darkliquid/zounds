package tags

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/darkliquid/zounds/pkg/core"
)

const pathTaggerVersion = "0.2.0"

var ignoredPathTokens = map[string]struct{}{
	"audio":   {},
	"clip":    {},
	"clips":   {},
	"file":    {},
	"files":   {},
	"fx":      {},
	"hit":     {},
	"hits":    {},
	"library": {},
	"loop":    {},
	"loops":   {},
	"one":     {},
	"pack":    {},
	"sample":  {},
	"samples": {},
	"shot":    {},
	"shots":   {},
	"sound":   {},
	"sounds":  {},
	"stem":    {},
	"stems":   {},
	"track":   {},
	"tracks":  {},
}

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
			if !isUsefulPathToken(normalized) {
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
	segment = strings.ReplaceAll(segment, "+", " ")
	if decoded, err := url.PathUnescape(segment); err == nil {
		segment = decoded
	}
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

func isUsefulPathToken(token string) bool {
	if token == "" {
		return false
	}
	if _, ignored := ignoredPathTokens[token]; ignored {
		return false
	}
	if len(token) < 2 {
		return false
	}
	if allDigits(token) {
		switch token {
		case "606", "707", "808", "909":
			return true
		default:
			return false
		}
	}
	return true
}

func allDigits(value string) bool {
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return value != ""
}
