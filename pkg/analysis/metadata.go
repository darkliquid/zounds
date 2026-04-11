package analysis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dhowden/tag"

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

type EmbeddedMetadata struct {
	Format     string
	FileType   string
	Values     map[string]string
	HasPicture bool
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
			"embedded_fields":  float64(len(embedded.Values)),
		},
		Attributes: attributes,
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

func ReadEmbeddedMetadata(path string) (EmbeddedMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return EmbeddedMetadata{}, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		if errors.Is(err, tag.ErrNoTagsFound) {
			return EmbeddedMetadata{}, nil
		}
		return EmbeddedMetadata{}, err
	}

	trackNumber, trackTotal := metadata.Track()
	discNumber, discTotal := metadata.Disc()

	values := map[string]string{}
	addMetadataValue(values, "title", metadata.Title())
	addMetadataValue(values, "album", metadata.Album())
	addMetadataValue(values, "artist", metadata.Artist())
	addMetadataValue(values, "album_artist", metadata.AlbumArtist())
	addMetadataValue(values, "composer", metadata.Composer())
	addMetadataValue(values, "genre", metadata.Genre())
	addMetadataValue(values, "comment", metadata.Comment())
	addMetadataValue(values, "lyrics", metadata.Lyrics())
	if metadata.Year() > 0 {
		values["year"] = fmt.Sprintf("%d", metadata.Year())
	}
	if trackNumber > 0 {
		values["track"] = fmt.Sprintf("%d", trackNumber)
	}
	if trackTotal > 0 {
		values["track_total"] = fmt.Sprintf("%d", trackTotal)
	}
	if discNumber > 0 {
		values["disc"] = fmt.Sprintf("%d", discNumber)
	}
	if discTotal > 0 {
		values["disc_total"] = fmt.Sprintf("%d", discTotal)
	}

	return EmbeddedMetadata{
		Format:     string(metadata.Format()),
		FileType:   string(metadata.FileType()),
		Values:     values,
		HasPicture: metadata.Picture() != nil,
	}, nil
}

func addMetadataValue(values map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[key] = value
}
