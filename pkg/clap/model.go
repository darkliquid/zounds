package clap

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

// Config holds all parameters for a CLAP Model.  Use DefaultConfig() and
// adjust only the fields you need.
type Config struct {
	// Feature extractor settings.
	FeatureExtractor FeatureExtractorConfig

	// Dimension of the audio / text embeddings produced by the ONNX models.
	// Defaults to DefaultEmbedDim (512) for laion/clap-htsat-*.
	EmbedDim int

	// Maximum token sequence length including BOS/EOS (default 77).
	MaxTokens int

	// File names inside the model directory.
	AudioModelFile string
	TextModelFile  string
	TokenizerFile  string

	// ONNX tensor input / output names.
	// Defaults match the HuggingFace optimum ONNX export of laion/clap-htsat-*.
	AudioInputName    string
	AudioOutputName   string
	TextIDsName       string
	TextMaskName      string
	TextOutputName    string

	// Path to the ONNX Runtime shared library (e.g. /usr/lib/libonnxruntime.so).
	// An empty string uses the platform default ("onnxruntime.so").
	LibPath string
}

// DefaultConfig returns a Config pre-filled with the defaults for
// laion/clap-htsat-unfused exported via optimum.
func DefaultConfig() Config {
	return Config{
		FeatureExtractor: DefaultFeatureExtractorConfig(),
		EmbedDim:         DefaultEmbedDim,
		MaxTokens:        DefaultMaxTokens,
		AudioModelFile:   "audio_model.onnx",
		TextModelFile:    "text_model.onnx",
		TokenizerFile:    "tokenizer.json",
		AudioInputName:   "input_features",
		AudioOutputName:  "audio_embeds",
		TextIDsName:      "input_ids",
		TextMaskName:     "attention_mask",
		TextOutputName:   "text_embeds",
	}
}

func applyZeroValueDefaults(dst any, defaults any) {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return
	}

	dv = dv.Elem()
	defv := reflect.ValueOf(defaults)
	if dv.Kind() != reflect.Struct || defv.Kind() != reflect.Struct || dv.Type() != defv.Type() {
		return
	}

	for i := 0; i < dv.NumField(); i++ {
		field := dv.Field(i)
		if !field.CanSet() {
			continue
		}
		if field.IsZero() {
			field.Set(defv.Field(i))
		}
	}
}

func (c *Config) applyDefaults() {
	d := DefaultConfig()
	if c.EmbedDim <= 0 {
		c.EmbedDim = d.EmbedDim
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = d.MaxTokens
	}
	if c.AudioModelFile == "" {
		c.AudioModelFile = d.AudioModelFile
	}
	if c.TextModelFile == "" {
		c.TextModelFile = d.TextModelFile
	}
	if c.TokenizerFile == "" {
		c.TokenizerFile = d.TokenizerFile
	}
	if c.AudioInputName == "" {
		c.AudioInputName = d.AudioInputName
	}
	if c.AudioOutputName == "" {
		c.AudioOutputName = d.AudioOutputName
	}
	if c.TextIDsName == "" {
		c.TextIDsName = d.TextIDsName
	}
	if c.TextMaskName == "" {
		c.TextMaskName = d.TextMaskName
	}
	if c.TextOutputName == "" {
		c.TextOutputName = d.TextOutputName
	}
	applyZeroValueDefaults(&c.FeatureExtractor, d.FeatureExtractor)
}

// LabelScore pairs a text label with its cosine similarity to the audio
// embedding.
type LabelScore struct {
	Label string
	Score float32
}

// Model is a loaded CLAP model that can classify audio against a set of text
// labels.
type Model struct {
	audio     *audioEncoder
	text      *textEncoder
	tokenizer *BPETokenizer
	extractor *FeatureExtractor
	cfg       Config
}

// NewModel loads a CLAP model from modelDir.  The directory must contain the
// ONNX model files and tokenizer.json as specified by cfg (defaults apply).
//
// The ONNX Runtime shared library must be present on the system.  Specify its
// path in cfg.LibPath or ensure it is discoverable via the platform default.
func NewModel(modelDir string, cfg Config) (*Model, error) {
	cfg.applyDefaults()

	if err := ensureORT(cfg.LibPath); err != nil {
		return nil, fmt.Errorf("initialise ONNX Runtime: %w", err)
	}

	extractor, err := NewFeatureExtractor(cfg.FeatureExtractor)
	if err != nil {
		return nil, fmt.Errorf("create feature extractor: %w", err)
	}

	numFrames := cfg.FeatureExtractor.NumFrames()
	numMelBins := cfg.FeatureExtractor.NumMelBins

	audio, err := newAudioEncoder(
		filepath.Join(modelDir, cfg.AudioModelFile),
		numMelBins, numFrames, cfg.EmbedDim,
		cfg.AudioInputName, cfg.AudioOutputName,
	)
	if err != nil {
		return nil, fmt.Errorf("load audio encoder: %w", err)
	}

	text, err := newTextEncoder(
		filepath.Join(modelDir, cfg.TextModelFile),
		cfg.MaxTokens, cfg.EmbedDim,
		cfg.TextIDsName, cfg.TextMaskName, cfg.TextOutputName,
	)
	if err != nil {
		_ = audio.close()
		return nil, fmt.Errorf("load text encoder: %w", err)
	}

	tokenizer, err := NewBPETokenizer(
		filepath.Join(modelDir, cfg.TokenizerFile),
		cfg.MaxTokens,
	)
	if err != nil {
		_ = audio.close()
		_ = text.close()
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	return &Model{
		audio:     audio,
		text:      text,
		tokenizer: tokenizer,
		extractor: extractor,
		cfg:       cfg,
	}, nil
}

// ClassifyAudio computes cosine similarity scores between the audio in buf and
// each text label, returning results sorted by score (highest first).
func (m *Model) ClassifyAudio(ctx context.Context, buf zaudio.PCMBuffer, labels []string) ([]LabelScore, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	melSpec, err := m.extractor.ExtractFromBuffer(buf)
	if err != nil {
		return nil, fmt.Errorf("extract mel spectrogram: %w", err)
	}

	audioEmb, err := m.audio.encode(melSpec)
	if err != nil {
		return nil, fmt.Errorf("audio encode: %w", err)
	}
	l2Normalize(audioEmb)

	scores := make([]LabelScore, 0, len(labels))
	for _, label := range labels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ids, mask := m.tokenizer.Encode(label)
		textEmb, err := m.text.encode(ids, mask)
		if err != nil {
			return nil, fmt.Errorf("text encode %q: %w", label, err)
		}
		l2Normalize(textEmb)

		scores = append(scores, LabelScore{
			Label: label,
			Score: cosineSimilarity(audioEmb, textEmb),
		})
	}

	slices.SortFunc(scores, func(a, b LabelScore) int {
		if a.Score > b.Score {
			return -1
		}
		if a.Score < b.Score {
			return 1
		}
		return 0
	})

	return scores, nil
}

// Close releases all resources held by the model.
func (m *Model) Close() error {
	var firstErr error
	for _, fn := range []func() error{m.audio.close, m.text.close} {
		if err := fn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
