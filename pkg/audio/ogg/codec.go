package ogg

import (
	"context"
	"fmt"
	"io"

	"github.com/jfreymuth/oggvorbis"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatOGG
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	samples, format, err := oggvorbis.ReadAll(reader)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode ogg/vorbis stream: %w", err)
	}
	if format == nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode ogg/vorbis: missing format information")
	}

	data := make([]float64, len(samples))
	for i, sample := range samples {
		data[i] = float64(sample)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: format.SampleRate,
		Channels:   format.Channels,
		BitDepth:   32,
		Data:       data,
		Metadata: map[string]string{
			"codec": "ogg/vorbis",
		},
	}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatOGG,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func Register(registry *zaudio.Registry) error {
	return registry.RegisterDecoder(New())
}
