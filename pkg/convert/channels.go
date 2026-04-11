package convert

import (
	"fmt"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

type ChannelMode string

const (
	ChannelModeAverage   ChannelMode = "average"
	ChannelModeDuplicate ChannelMode = "duplicate"
)

type ChannelOptions struct {
	TargetChannels int
	MonoMode       ChannelMode
	StereoMode     ChannelMode
}

func ConvertChannels(buffer zaudio.PCMBuffer, opts ChannelOptions) (zaudio.PCMBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return zaudio.PCMBuffer{}, fmt.Errorf("convert channels: %w", err)
	}

	if opts.TargetChannels <= 0 {
		return zaudio.PCMBuffer{}, fmt.Errorf("convert channels: target channels must be greater than zero")
	}

	if opts.TargetChannels == buffer.Channels {
		clone := buffer.Clone()
		clone.Channels = opts.TargetChannels
		return clone, nil
	}

	switch {
	case buffer.Channels == 2 && opts.TargetChannels == 1:
		return stereoToMono(buffer, opts)
	case buffer.Channels == 1 && opts.TargetChannels == 2:
		return monoToStereo(buffer, opts)
	default:
		return zaudio.PCMBuffer{}, fmt.Errorf("convert channels: unsupported channel conversion %d -> %d", buffer.Channels, opts.TargetChannels)
	}
}

func stereoToMono(buffer zaudio.PCMBuffer, opts ChannelOptions) (zaudio.PCMBuffer, error) {
	mode := opts.MonoMode
	if mode == "" {
		mode = ChannelModeAverage
	}
	if mode != ChannelModeAverage {
		return zaudio.PCMBuffer{}, fmt.Errorf("convert channels: unsupported mono mix mode %q", mode)
	}

	output := zaudio.PCMBuffer{
		SampleRate: buffer.SampleRate,
		Channels:   1,
		BitDepth:   buffer.BitDepth,
		Data:       make([]float64, 0, buffer.Frames()),
		Metadata:   buffer.Clone().Metadata,
	}

	for i := 0; i < len(buffer.Data); i += 2 {
		output.Data = append(output.Data, (buffer.Data[i]+buffer.Data[i+1])*0.5)
	}

	return output, nil
}

func monoToStereo(buffer zaudio.PCMBuffer, opts ChannelOptions) (zaudio.PCMBuffer, error) {
	mode := opts.StereoMode
	if mode == "" {
		mode = ChannelModeDuplicate
	}
	if mode != ChannelModeDuplicate {
		return zaudio.PCMBuffer{}, fmt.Errorf("convert channels: unsupported stereo fill mode %q", mode)
	}

	output := zaudio.PCMBuffer{
		SampleRate: buffer.SampleRate,
		Channels:   2,
		BitDepth:   buffer.BitDepth,
		Data:       make([]float64, 0, len(buffer.Data)*2),
		Metadata:   buffer.Clone().Metadata,
	}

	for _, sample := range buffer.Data {
		output.Data = append(output.Data, sample, sample)
	}

	return output, nil
}
