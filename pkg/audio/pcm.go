package audio

import (
	"fmt"
	"math"

	rawaudio "github.com/go-audio/audio"
)

func IntBufferToPCM(buffer *rawaudio.IntBuffer, bitDepth int) (PCMBuffer, error) {
	if buffer == nil || buffer.Format == nil {
		return PCMBuffer{}, fmt.Errorf("int buffer is missing format information")
	}

	if bitDepth <= 0 {
		bitDepth = buffer.SourceBitDepth
	}
	if bitDepth <= 0 {
		bitDepth = 16
	}

	scale := math.Pow(2, float64(bitDepth-1))
	data := make([]float64, len(buffer.Data))
	for i, sample := range buffer.Data {
		data[i] = clamp(float64(sample)/scale, -1, 1)
	}

	return PCMBuffer{
		SampleRate: buffer.Format.SampleRate,
		Channels:   buffer.Format.NumChannels,
		BitDepth:   bitDepth,
		Data:       data,
	}, nil
}

func PCMToIntBuffer(buffer PCMBuffer) (*rawaudio.IntBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return nil, err
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

	return &rawaudio.IntBuffer{
		Format: &rawaudio.Format{
			NumChannels: buffer.Channels,
			SampleRate:  buffer.SampleRate,
		},
		SourceBitDepth: bitDepth,
		Data:           ints,
	}, nil
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
