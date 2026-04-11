package wav

import (
	"context"
	"fmt"
	"io"
	"math"

	rawaudio "github.com/go-audio/audio"
	gowav "github.com/go-audio/wav"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatWAV
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	decoder := gowav.NewDecoder(reader)
	if !decoder.IsValidFile() {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: invalid file")
	}

	intBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav pcm buffer: %w", err)
	}
	if intBuffer == nil || intBuffer.Format == nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: missing PCM format information")
	}

	bitDepth := intBuffer.SourceBitDepth
	if bitDepth <= 0 {
		bitDepth = 16
	}

	scale := math.Pow(2, float64(bitDepth-1))
	data := make([]float64, len(intBuffer.Data))
	for i, sample := range intBuffer.Data {
		data[i] = clamp(float64(sample)/scale, -1, 1)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: intBuffer.Format.SampleRate,
		Channels:   intBuffer.Format.NumChannels,
		BitDepth:   bitDepth,
		Data:       data,
		Metadata: map[string]string{
			"codec": "wav",
		},
	}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatWAV,
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

	if err := buffer.Validate(); err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}

	bitDepth := buffer.BitDepth
	if bitDepth <= 0 {
		bitDepth = 16
	}

	maxInt := math.Pow(2, float64(bitDepth-1)) - 1
	minInt := -math.Pow(2, float64(bitDepth-1))

	ints := make([]int, len(buffer.Data))
	for i, sample := range buffer.Data {
		value := math.Round(sample * maxInt)
		value = clamp(value, minInt, maxInt)
		ints[i] = int(value)
	}

	intBuffer := &rawaudio.IntBuffer{
		Format: &rawaudio.Format{
			NumChannels: buffer.Channels,
			SampleRate:  buffer.SampleRate,
		},
		SourceBitDepth: bitDepth,
		Data:           ints,
	}

	encoder := gowav.NewEncoder(writer, buffer.SampleRate, bitDepth, buffer.Channels, 1)
	if err := encoder.Write(intBuffer); err != nil {
		return fmt.Errorf("encode wav buffer: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finalize wav file: %w", err)
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

func clamp[T ~float64](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
