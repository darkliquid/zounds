package flac

import (
	"context"
	"fmt"
	"io"
	"math"

	goflac "github.com/mewkiz/flac"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatFLAC
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	stream, err := goflac.NewSeek(reader)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode flac stream: %w", err)
	}
	if stream.Info == nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode flac: missing stream info")
	}

	bitDepth := int(stream.Info.BitsPerSample)
	channels := int(stream.Info.NChannels)
	if bitDepth <= 0 {
		bitDepth = 16
	}
	if channels <= 0 {
		return zaudio.DecodeResult{}, fmt.Errorf("decode flac: invalid channel count")
	}

	var data []float64
	for {
		select {
		case <-ctx.Done():
			return zaudio.DecodeResult{}, ctx.Err()
		default:
		}

		frame, err := stream.ParseNext()
		if err != nil {
			if err == io.EOF {
				break
			}
			return zaudio.DecodeResult{}, fmt.Errorf("decode flac frame: %w", err)
		}
		if len(frame.Subframes) == 0 {
			continue
		}

		samplesPerChannel := len(frame.Subframes[0].Samples)
		for i := 0; i < samplesPerChannel; i++ {
			for ch := 0; ch < len(frame.Subframes); ch++ {
				data = append(data, normalizePCM(frame.Subframes[ch].Samples[i], bitDepth))
			}
		}
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: int(stream.Info.SampleRate),
		Channels:   channels,
		BitDepth:   bitDepth,
		Data:       data,
		Metadata: map[string]string{
			"codec": "flac",
		},
	}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatFLAC,
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

func normalizePCM(sample int32, bitDepth int) float64 {
	scale := math.Pow(2, float64(bitDepth-1))
	value := float64(sample) / scale
	if value < -1 {
		return -1
	}
	if value > 1 {
		return 1
	}
	return value
}
