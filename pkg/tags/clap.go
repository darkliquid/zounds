package tags

import (
	"context"
	"fmt"
	"slices"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/clap"
	"github.com/darkliquid/zounds/pkg/core"
)

const localCLAPTaggerVersion = "0.1.0"

const (
	defaultCLAPMinScore     = 0.32
	defaultCLAPMaxPredicted = 3
	defaultCLAPScoreSlack   = 0.05
)

// defaultCLAPLabels is the built-in set of audio descriptors used when the
// caller does not supply an explicit label list.
var defaultCLAPLabels = []string{
	"bass", "bell", "clap", "drone", "glitch", "hi hat", "impact",
	"kick", "lead", "metallic", "pad", "percussive", "pluck",
	"snare", "sub bass", "sweep", "vocal chop",
}

// LocalCLAPTagger tags audio samples using a locally hosted CLAP ONNX model.
// It requires:
//   - A directory containing audio_model.onnx, text_model.onnx, and
//     tokenizer.json (as exported by optimum from laion/clap-htsat-unfused or
//     a compatible model).
//   - The ONNX Runtime shared library installed on the system (or explicitly
//     specified via libPath).
type LocalCLAPTagger struct {
	model        *clap.Model
	registry     *zaudio.Registry
	labels       []string
	defaultsUsed bool
	minScore     float32
	maxPredicted int
}

// NewLocalCLAPTagger creates a LocalCLAPTagger.
//
// modelDir is the path to the directory containing the ONNX models and
// tokenizer.json.
//
// libPath is the path to the ONNX Runtime shared library; leave empty to use
// the platform default ("onnxruntime.so").
//
// labels is the set of text labels to score against; nil/empty uses
// defaultCLAPLabels.
//
// minScore is the minimum cosine similarity [0,1] for a tag to be emitted
// (default 0.2).
//
// maxPredicted caps how many tags are returned per sample (default 5).
func NewLocalCLAPTagger(modelDir, libPath string, labels []string, minScore float32, maxPredicted int) (*LocalCLAPTagger, error) {
	defaultsUsed := len(labels) == 0
	if len(labels) == 0 {
		labels = append([]string(nil), defaultCLAPLabels...)
	} else {
		labels = append([]string(nil), labels...)
	}
	if minScore <= 0 {
		minScore = defaultCLAPMinScore
	}
	if maxPredicted <= 0 {
		maxPredicted = defaultCLAPMaxPredicted
	}

	cfg := clap.DefaultConfig()
	cfg.LibPath = libPath

	model, err := clap.NewModel(modelDir, cfg)
	if err != nil {
		return nil, fmt.Errorf("load CLAP model from %q: %w", modelDir, err)
	}

	registry, err := codecs.NewRegistry()
	if err != nil {
		_ = model.Close()
		return nil, fmt.Errorf("create codec registry: %w", err)
	}

	return &LocalCLAPTagger{
		model:        model,
		registry:     registry,
		labels:       labels,
		defaultsUsed: defaultsUsed,
		minScore:     minScore,
		maxPredicted: maxPredicted,
	}, nil
}

// Close releases all resources held by the tagger.
func (t *LocalCLAPTagger) Close() error {
	return t.model.Close()
}

// Name implements core.Tagger.
func (LocalCLAPTagger) Name() string { return "clap" }

// Version implements core.Tagger.
func (LocalCLAPTagger) Version() string { return localCLAPTaggerVersion }

// Tags implements core.Tagger.  It decodes the sample, runs CLAP inference,
// and returns the top matching labels as core.Tag values.
func (t *LocalCLAPTagger) Tags(ctx context.Context, sample core.Sample, _ core.AnalysisResult) ([]core.Tag, error) {
	decoded, err := zaudio.DecodeFile(ctx, t.registry, sample.Path)
	if err != nil {
		return nil, fmt.Errorf("decode %q for CLAP: %w", sample.Path, err)
	}

	scores, err := t.model.ClassifyAudio(ctx, decoded.Buffer, t.labels)
	if err != nil {
		return nil, fmt.Errorf("CLAP classify %q: %w", sample.Path, err)
	}

	return t.tagsForScores(scores), nil
}

func (t *LocalCLAPTagger) tagsForScores(scores []clap.LabelScore) []core.Tag {
	out := make([]core.Tag, 0, t.maxPredicted)
	scoreFloor := t.minScore
	if t.defaultsUsed && len(scores) > 0 {
		scoreFloor = maxFloat32(scoreFloor, scores[0].Score-defaultCLAPScoreSlack)
	}
	for _, s := range scores {
		if len(out) >= t.maxPredicted {
			break
		}
		if s.Score < scoreFloor {
			break // scores are sorted descending, so we can stop early
		}
		name := core.NormalizeTagName(s.Label)
		if name == "" {
			continue
		}
		out = append(out, core.Tag{
			Name:           name,
			NormalizedName: name,
			Source:         "clap",
			Confidence:     float64(s.Score),
		})
	}
	slices.SortFunc(out, func(left, right core.Tag) int {
		switch {
		case left.Confidence > right.Confidence:
			return -1
		case left.Confidence < right.Confidence:
			return 1
		case left.NormalizedName < right.NormalizedName:
			return -1
		case left.NormalizedName > right.NormalizedName:
			return 1
		default:
			return 0
		}
	})
	return out
}

func maxFloat32(left, right float32) float32 {
	if left > right {
		return left
	}
	return right
}
