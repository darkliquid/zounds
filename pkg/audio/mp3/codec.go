package mp3

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	gomp3 "github.com/hajimehoshi/go-mp3"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatMP3
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	decoder, err := gomp3.NewDecoder(reader)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode mp3 stream: %w", err)
	}

	raw, err := io.ReadAll(decoder)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("read decoded mp3 pcm: %w", err)
	}

	data := make([]float64, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(raw[i : i+2]))
		data = append(data, float64(sample)/32768.0)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: decoder.SampleRate(),
		Channels:   2,
		BitDepth:   16,
		Data:       data,
		Metadata: map[string]string{
			"codec": "mp3",
		},
	}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatMP3,
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
