package tags

import (
	"context"
	"fmt"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/clap"
	"github.com/darkliquid/zounds/pkg/core"
)

const localCLAPTaggerVersion = "0.1.0"

// defaultCLAPLabels is the built-in set of audio descriptors used when the
// caller does not supply an explicit label list.
var defaultCLAPLabels = []string{
	"ambient", "analogue", "atmospheric", "bass", "bell", "bright",
	"cinematic", "classical", "cyberpunk", "dark", "distorted", "drone",
	"epic", "glitch", "industrial", "lead", "lofi", "metallic", "pad",
	"percussive", "plucked", "retro", "sub", "texture", "vocal",
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
	if len(labels) == 0 {
		labels = append([]string(nil), defaultCLAPLabels...)
	} else {
		labels = append([]string(nil), labels...)
	}
	if minScore <= 0 {
		minScore = 0.2
	}
	if maxPredicted <= 0 {
		maxPredicted = 5
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

	out := make([]core.Tag, 0, t.maxPredicted)
	for _, s := range scores {
		if len(out) >= t.maxPredicted {
			break
		}
		if s.Score < t.minScore {
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
	return out, nil
}
