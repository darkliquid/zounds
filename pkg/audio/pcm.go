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

func Unsigned8IntBufferToPCM(buffer *rawaudio.IntBuffer) (PCMBuffer, error) {
	if buffer == nil || buffer.Format == nil {
		return PCMBuffer{}, fmt.Errorf("int buffer is missing format information")
	}

	data := make([]float64, len(buffer.Data))
	for i, sample := range buffer.Data {
		u8 := clamp(float64(sample), 0, 255)
		data[i] = clamp((u8-128.0)/128.0, -1, 1)
	}

	return PCMBuffer{
		SampleRate: buffer.Format.SampleRate,
		Channels:   buffer.Format.NumChannels,
		BitDepth:   8,
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

	scale := math.Pow(2, float64(bitDepth-1))
	maxInt := scale - 1
	minInt := -scale

	ints := make([]int, len(buffer.Data))
	for i, sample := range buffer.Data {
		var value float64
		if sample < 0 {
			value = math.Round(sample * scale)
		} else {
			value = math.Round(sample * maxInt)
		}
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

func PCMToUnsigned8IntBuffer(buffer PCMBuffer) (*rawaudio.IntBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return nil, err
	}

	ints := make([]int, len(buffer.Data))
	for i, sample := range buffer.Data {
		value := clamp(math.Round(sample*128.0+128.0), 0, 255)
		ints[i] = int(value)
	}

	return &rawaudio.IntBuffer{
		Format: &rawaudio.Format{
			NumChannels: buffer.Channels,
			SampleRate:  buffer.SampleRate,
		},
		SourceBitDepth: 8,
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
