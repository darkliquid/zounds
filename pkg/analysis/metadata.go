package analysis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
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

type EmbeddedMetadata struct {
	Format     string
	FileType   string
	Values     map[string]string
	HasPicture bool
}

func NewMetadataAnalyzer(registry *zaudio.Registry) (*MetadataAnalyzer, error) {
	var err error
	registry, err = defaultRegistry(registry)
	if err != nil {
		return nil, err
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

	decoded, err := decodeSample(ctx, a.registry, sample)
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

	embedded, err := ReadEmbeddedMetadata(sample.Path)
	if err != nil {
		return core.AnalysisResult{}, fmt.Errorf("read embedded metadata for %q: %w", sample.Path, err)
	}

	attributes := map[string]string{
		"format":         string(metadata.Format),
		"channel_layout": metadata.ChannelLayout,
		"extension":      sample.Extension,
	}
	for key, value := range embedded.Values {
		attributes["embedded."+key] = value
	}
	if embedded.Format != "" {
		attributes["embedded.format"] = embedded.Format
	}
	if embedded.FileType != "" {
		attributes["embedded.file_type"] = embedded.FileType
	}
	if embedded.HasPicture {
		attributes["embedded.picture"] = "true"
	}

	metrics := map[string]float64{
		"sample_rate":      float64(metadata.SampleRate),
		"channels":         float64(metadata.Channels),
		"bit_depth":        float64(metadata.BitDepth),
		"bitrate":          float64(metadata.Bitrate),
		"frames":           float64(metadata.Frames),
		"duration_seconds": metadata.Duration.Seconds(),
		"size_bytes":       float64(metadata.SizeBytes),
		"embedded_fields":  float64(len(embedded.Values)),
	}

	// Apply embedded metadata as high-confidence priors
	if bpmStr, ok := embedded.Values["bpm"]; ok {
		if bpm, err := strconv.ParseFloat(bpmStr, 64); err == nil && bpm > 0 {
			metrics["tempo_bpm"] = bpm
			metrics["tempo_confidence"] = 1.0
			attributes["tempo_bpm"] = fmt.Sprintf("%.2f", bpm)
		}
	}
	if key, ok := embedded.Values["key"]; ok && key != "" {
		attributes["key"] = key
		if tonic, ok := embedded.Values["tonic"]; ok {
			attributes["tonic"] = tonic
		}
		if mode, ok := embedded.Values["mode"]; ok {
			attributes["mode"] = mode
		}
		metrics["key_confidence"] = 1.0
	}
	if loopClass, ok := embedded.Values["loop_class"]; ok && loopClass != "" {
		attributes["loop_class"] = loopClass
		switch loopClass {
		case "loop":
			metrics["loop_confidence"] = 1.0
		case "oneshot":
			metrics["loop_confidence"] = 0.0
		}
	}
	if midiStr, ok := embedded.Values["midi_note"]; ok {
		if note, err := strconv.Atoi(midiStr); err == nil && note >= 0 && note <= 127 {
			metrics["midi_note"] = float64(note)
			metrics["confidence"] = 1.0
			if noteName, ok := embedded.Values["note_name"]; ok {
				attributes["note_name"] = noteName
			}
			hz := MidiToHz(note)
			if hz > 0 {
				metrics["frequency_hz"] = hz
			}
		}
	}

	return core.AnalysisResult{
		SampleID:    sample.ID,
		Analyzer:    a.Name(),
		Version:     a.Version(),
		CompletedAt: time.Now().UTC(),
		Metrics:     metrics,
		Attributes:  attributes,
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
