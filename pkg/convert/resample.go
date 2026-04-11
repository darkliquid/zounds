package convert

import (
	"fmt"
	"math"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

type ResampleOptions struct {
	TargetSampleRate int
}

func ResampleLinear(buffer zaudio.PCMBuffer, opts ResampleOptions) (zaudio.PCMBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return zaudio.PCMBuffer{}, fmt.Errorf("resample: %w", err)
	}
	if opts.TargetSampleRate <= 0 {
		return zaudio.PCMBuffer{}, fmt.Errorf("resample: target sample rate must be greater than zero")
	}
	if opts.TargetSampleRate == buffer.SampleRate {
		clone := buffer.Clone()
		clone.SampleRate = opts.TargetSampleRate
		return clone, nil
	}

	inputFrames := buffer.Frames()
	outputFrames := int(math.Round(float64(inputFrames) * float64(opts.TargetSampleRate) / float64(buffer.SampleRate)))
	if outputFrames < 1 {
		outputFrames = 1
	}

	output := zaudio.PCMBuffer{
		SampleRate: opts.TargetSampleRate,
		Channels:   buffer.Channels,
		BitDepth:   buffer.BitDepth,
		Data:       make([]float64, outputFrames*buffer.Channels),
		Metadata:   buffer.Clone().Metadata,
	}

	for frame := 0; frame < outputFrames; frame++ {
		sourcePos := float64(frame) * float64(buffer.SampleRate) / float64(opts.TargetSampleRate)
		left := int(math.Floor(sourcePos))
		if left >= inputFrames {
			left = inputFrames - 1
		}
		right := left + 1
		if right >= inputFrames {
			right = inputFrames - 1
		}
		weight := sourcePos - float64(left)

		for channel := 0; channel < buffer.Channels; channel++ {
			leftSample := buffer.Data[left*buffer.Channels+channel]
			rightSample := buffer.Data[right*buffer.Channels+channel]
			output.Data[frame*buffer.Channels+channel] = leftSample + (rightSample-leftSample)*weight
		}
	}

	return output, nil
}
