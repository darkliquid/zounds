package tags

import (
	"context"
	"fmt"
	"slices"
	"sync"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/clap"
	"github.com/darkliquid/zounds/pkg/core"
)

const localCLAPTaggerVersion = "0.2.0"

const (
	defaultCLAPMinScore     = 0.20
	defaultCLAPMaxPredicted = 4
	defaultCLAPScoreSlack   = 0.08
)

// defaultCLAPLabels is the built-in namespaced faceted taxonomy used when the
// caller does not supply an explicit label list.
var defaultCLAPLabels = DefaultFacetedLabels

// LocalCLAPTagger tags audio samples using a locally hosted CLAP ONNX model with
// prompt ensembling across the faceted tag taxonomy.
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
	ensembles    map[string][]float32
	defaultsUsed bool
	minScore     float32
	maxPredicted int
	mu           sync.Mutex
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
		ensembles:    make(map[string][]float32, len(labels)),
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
func (*LocalCLAPTagger) Name() string { return "clap" }

// Version implements core.Tagger.
func (*LocalCLAPTagger) Version() string { return localCLAPTaggerVersion }

// Tags implements core.Tagger. It decodes the sample, computes the audio
// embedding, matches against prompt-ensembled text representations, and
// returns the top matching labels as core.Tag values.
func (t *LocalCLAPTagger) Tags(ctx context.Context, sample core.Sample, _ core.AnalysisResult) ([]core.Tag, error) {
	decoded, err := zaudio.DecodeFile(ctx, t.registry, sample.Path)
	if err != nil {
		return nil, fmt.Errorf("decode %q for CLAP: %w", sample.Path, err)
	}

	audioEmb, err := t.model.EmbedAudio(ctx, decoded.Buffer)
	if err != nil {
		return nil, fmt.Errorf("CLAP audio embed %q: %w", sample.Path, err)
	}

	scores := make([]clap.LabelScore, 0, len(t.labels))
	for _, label := range t.labels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ensEmb, err := t.getEnsembleEmbedding(ctx, label)
		if err != nil {
			return nil, fmt.Errorf("CLAP ensemble embed %q: %w", label, err)
		}

		scores = append(scores, clap.LabelScore{
			Label: label,
			Score: clap.CosineSimilarity(audioEmb, ensEmb),
		})
	}

	slices.SortFunc(scores, func(a, b clap.LabelScore) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	return t.tagsForScores(scores), nil
}

func (t *LocalCLAPTagger) getEnsembleEmbedding(ctx context.Context, label string) ([]float32, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.ensembles == nil {
		t.ensembles = make(map[string][]float32)
	}
	if emb, ok := t.ensembles[label]; ok {
		return emb, nil
	}

	emb, err := EnsembleTextEmbedding(ctx, t.model, label)
	if err != nil {
		return nil, err
	}
	t.ensembles[label] = emb
	return emb, nil
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
