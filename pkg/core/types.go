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
	ID           int64
	SourceRoot   string
	Path         string
	RelativePath string
	FileName     string
	Extension    string
	Format       AudioFormat
	SizeBytes    int64
	ModifiedAt   time.Time
	ScannedAt    time.Time
	Metadata     map[string]string
}

type Tag struct {
	ID             int64
	Name           string
	NormalizedName string
	Source         string
	Confidence     float64
	CreatedAt      time.Time
}

type FeatureVector struct {
	ID         int64
	SampleID   int64
	Namespace  string
	Version    string
	Values     []float64
	Dimensions int
	CreatedAt  time.Time
}

type Cluster struct {
	ID         int64
	Method     string
	Label      string
	Size       int
	Parameters map[string]float64
	CreatedAt  time.Time
}

type AnalysisResult struct {
	SampleID       int64
	Analyzer       string
	Version        string
	CompletedAt    time.Time
	Metrics        map[string]float64
	Attributes     map[string]string
	Tags           []Tag
	FeatureVectors []FeatureVector
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
	return strings.Join(strings.Fields(name), " ")
}
