package core

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

type AudioFormat string

const (
	FormatUnknown AudioFormat = "unknown"
	FormatWAV     AudioFormat = "wav"
	FormatAIFF    AudioFormat = "aiff"
	FormatMP3     AudioFormat = "mp3"
	FormatFLAC    AudioFormat = "flac"
	FormatOGG     AudioFormat = "ogg"
	FormatAAC     AudioFormat = "aac"
	FormatM4A     AudioFormat = "m4a"
)

type Sample struct {
	ID           int64             `json:"id"`
	SourceRoot   string            `json:"source_root,omitempty"`
	Path         string            `json:"path"`
	RelativePath string            `json:"relative_path,omitempty"`
	FileName     string            `json:"file_name"`
	Extension    string            `json:"extension,omitempty"`
	Format       AudioFormat       `json:"format"`
	SizeBytes    int64             `json:"size_bytes"`
	ModifiedAt   time.Time         `json:"modified_at"`
	ScannedAt    time.Time         `json:"scanned_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Tag struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	NormalizedName string    `json:"normalized_name"`
	Source         string    `json:"source,omitempty"`
	Confidence     float64   `json:"confidence"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
}

type FeatureVector struct {
	ID         int64     `json:"id,omitempty"`
	SampleID   int64     `json:"sample_id"`
	Namespace  string    `json:"namespace"`
	Version    string    `json:"version,omitempty"`
	Values     []float64 `json:"values"`
	Dimensions int       `json:"dimensions"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
}

type Cluster struct {
	ID         int64              `json:"id"`
	Method     string             `json:"method"`
	Label      string             `json:"label"`
	Size       int                `json:"size"`
	Parameters map[string]float64 `json:"parameters,omitempty"`
	CreatedAt  time.Time          `json:"created_at,omitempty"`
}

type AnalysisResult struct {
	SampleID       int64              `json:"sample_id"`
	Analyzer       string             `json:"analyzer"`
	Version        string             `json:"version"`
	CompletedAt    time.Time          `json:"completed_at"`
	Metrics        map[string]float64 `json:"metrics,omitempty"`
	Attributes     map[string]string  `json:"attributes,omitempty"`
	Tags           []Tag              `json:"tags,omitempty"`
	FeatureVectors []FeatureVector    `json:"feature_vectors,omitempty"`
}

// SampleAnalysis is the persisted, per-sample flattening of analyzer output.
// Numeric fields are pointers because the analyzers do not always produce
// every measurement (e.g. key confidence on unpitched percussion).
type SampleAnalysis struct {
	SampleID           int64              `json:"sample_id"`
	DurationSeconds    *float64           `json:"duration_seconds,omitempty"`
	SampleRate         *int64             `json:"sample_rate,omitempty"`
	Channels           *int64             `json:"channels,omitempty"`
	BitDepth           *int64             `json:"bit_depth,omitempty"`
	BPM                *float64           `json:"bpm,omitempty"`
	BPMConfidence      *float64           `json:"bpm_confidence,omitempty"`
	KeyTonic           *string            `json:"key_tonic,omitempty"`
	KeyMode            *string            `json:"key_mode,omitempty"`
	KeyConfidence      *float64           `json:"key_confidence,omitempty"`
	Chord              *string            `json:"chord,omitempty"`
	ChordConfidence    *float64           `json:"chord_confidence,omitempty"`
	PitchHz            *float64           `json:"pitch_hz,omitempty"`
	MidiNote           *int64             `json:"midi_note,omitempty"`
	CentsOffset        *float64           `json:"cents_offset,omitempty"`
	PitchConfidence    *float64           `json:"pitch_confidence,omitempty"`
	LoopClass          *string            `json:"loop_class,omitempty"`
	LoopConfidence     *float64           `json:"loop_confidence,omitempty"`
	LUFSIntegrated     *float64           `json:"lufs_integrated,omitempty"`
	TruePeakDBTP       *float64           `json:"true_peak_dbtp,omitempty"`
	AnalyzedModifiedAt time.Time          `json:"analyzed_modified_at,omitempty"`
	Metrics            map[string]float64 `json:"metrics,omitempty"`
	Attributes         map[string]string  `json:"attributes,omitempty"`
	CreatedAt          time.Time          `json:"created_at,omitempty"`
	UpdatedAt          time.Time          `json:"updated_at,omitempty"`
}

// SampleProjection is a persisted 2D coordinate for a sample produced by a
// dimensionality-reduction method over a feature-vector namespace.
type SampleProjection struct {
	SampleID  int64     `json:"sample_id"`
	Method    string    `json:"method"`
	Namespace string    `json:"namespace"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// PerceptualHash is a persisted perceptual fingerprint for a sample.
type PerceptualHash struct {
	SampleID  int64     `json:"sample_id"`
	Algorithm string    `json:"algorithm"`
	Hash      string    `json:"hash"`
	HashBits  int       `json:"hash_bits"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type Analyzer interface {
	Name() string
	Version() string
	Analyze(ctx context.Context, sample Sample) (AnalysisResult, error)
}

type Tagger interface {
	Name() string
	Version() string
	Tags(ctx context.Context, sample Sample, result AnalysisResult) ([]Tag, error)
}

type Clusterer interface {
	Name() string
	Cluster(ctx context.Context, vectors []FeatureVector) ([]Cluster, error)
}

func DetectFormatFromExtension(path string) AudioFormat {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	switch ext {
	case "wav":
		return FormatWAV
	case "aif", "aiff":
		return FormatAIFF
	case "mp3":
		return FormatMP3
	case "flac":
		return FormatFLAC
	case "ogg", "oga":
		return FormatOGG
	case "aac":
		return FormatAAC
	case "m4a":
		return FormatM4A
	default:
		return FormatUnknown
	}
}

func NormalizeTagName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	if !strings.Contains(name, ":") {
		return strings.Join(strings.Fields(name), " ")
	}

	parts := strings.Split(name, ":")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		segment := strings.Join(strings.Fields(part), " ")
		if segment != "" {
			cleaned = append(cleaned, segment)
		}
	}
	return strings.Join(cleaned, ":")
}
