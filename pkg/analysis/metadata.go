package analysis

import (
	"context"
	"fmt"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
)

const metadataAnalyzerVersion = "0.1.0"

type MetadataAnalyzer struct {
	registry *zaudio.Registry
}

type AudioMetadata struct {
	Format        core.AudioFormat
	SampleRate    int
	Channels      int
	BitDepth      int
	Bitrate       int
	Frames        int
	Duration      time.Duration
	SizeBytes     int64
	ChannelLayout string
}

func NewMetadataAnalyzer(registry *zaudio.Registry) (*MetadataAnalyzer, error) {
	if registry == nil {
		var err error
		registry, err = codecs.NewRegistry()
		if err != nil {
			return nil, fmt.Errorf("create default codec registry: %w", err)
		}
	}

	return &MetadataAnalyzer{registry: registry}, nil
}

func (a *MetadataAnalyzer) Name() string {
	return "metadata"
}

func (a *MetadataAnalyzer) Version() string {
	return metadataAnalyzerVersion
}

func (a *MetadataAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if a == nil || a.registry == nil {
		return core.AnalysisResult{}, fmt.Errorf("metadata analyzer is not initialized")
	}

	decoded, err := zaudio.DecodeFile(ctx, a.registry, sample.Path)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("analyze metadata for %q: %w", sample.Path, err)
	}

	metadata := AudioMetadata{
		Format:        decoded.Info.Format,
		SampleRate:    decoded.Info.SampleRate,
		Channels:      decoded.Info.Channels,
		BitDepth:      decoded.Info.BitDepth,
		Bitrate:       calculateBitrate(sample.SizeBytes, decoded.Buffer.Duration()),
		Frames:        decoded.Buffer.Frames(),
		Duration:      decoded.Buffer.Duration(),
		SizeBytes:     sample.SizeBytes,
		ChannelLayout: channelLayout(decoded.Info.Channels),
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics: map[string]float64{
			"sample_rate":      float64(metadata.SampleRate),
			"channels":         float64(metadata.Channels),
			"bit_depth":        float64(metadata.BitDepth),
			"bitrate":          float64(metadata.Bitrate),
			"frames":           float64(metadata.Frames),
			"duration_seconds": metadata.Duration.Seconds(),
			"size_bytes":       float64(metadata.SizeBytes),
		},
		Attributes: map[string]string{
			"format":         string(metadata.Format),
			"channel_layout": metadata.ChannelLayout,
			"extension":      sample.Extension,
		},
	}, nil
}

func calculateBitrate(sizeBytes int64, duration time.Duration) int {
	if sizeBytes <= 0 || duration <= 0 {
		return 0
	}
	return int(float64(sizeBytes*8) / duration.Seconds())
}

func channelLayout(channels int) string {
	switch channels {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	default:
		return fmt.Sprintf("%d-channel", channels)
	}
}
