package aiff

import (
	"context"
	"fmt"
	"io"

	goaiff "github.com/go-audio/aiff"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatAIFF
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	decoder := goaiff.NewDecoder(reader)
	if !decoder.IsValidFile() {
		return zaudio.DecodeResult{}, fmt.Errorf("decode aiff: invalid file")
	}

	intBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode aiff pcm buffer: %w", err)
	}

	buffer, err := zaudio.IntBufferToPCM(intBuffer, int(decoder.SampleBitDepth()))
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode aiff pcm conversion: %w", err)
	}

	buffer.Metadata = map[string]string{
		"codec": "aiff",
	}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatAIFF,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func (Codec) Encode(ctx context.Context, writer io.WriteSeeker, buffer zaudio.PCMBuffer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	intBuffer, err := zaudio.PCMToIntBuffer(buffer)
	if err != nil {
		return fmt.Errorf("encode aiff: %w", err)
	}

	encoder := goaiff.NewEncoder(writer, buffer.SampleRate, intBuffer.SourceBitDepth, buffer.Channels)
	if err := encoder.Write(intBuffer); err != nil {
		return fmt.Errorf("encode aiff buffer: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finalize aiff file: %w", err)
	}

	return nil
}

func Register(registry *zaudio.Registry) error {
	codec := New()
	if err := registry.RegisterDecoder(codec); err != nil {
		return err
	}
	if err := registry.RegisterEncoder(codec); err != nil {
		return err
	}
	return nil
}
