package convert_test

import (
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/convert"
)

func TestConvertChannelsStereoToMonoAveragesFrames(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 48000,
		Channels:   2,
		BitDepth:   16,
		Data:       []float64{0.5, -0.5, 1.0, 0.0, -0.25, -0.75},
	}

	converted, err := convert.ConvertChannels(buffer, convert.ChannelOptions{TargetChannels: 1})
	if err != nil {
		t.Fatalf("convert stereo to mono: %v", err)
	}

	if converted.Channels != 1 {
		t.Fatalf("expected 1 channel, got %d", converted.Channels)
	}

	expected := []float64{0, 0.5, -0.5}
	for i, want := range expected {
		if converted.Data[i] != want {
			t.Fatalf("frame %d: expected %f, got %f", i, want, converted.Data[i])
		}
	}
}

func TestConvertChannelsMonoToStereoDuplicatesFrames(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   24,
		Data:       []float64{-0.25, 0.0, 0.75},
	}

	converted, err := convert.ConvertChannels(buffer, convert.ChannelOptions{TargetChannels: 2})
	if err != nil {
		t.Fatalf("convert mono to stereo: %v", err)
	}

	if converted.Channels != 2 {
		t.Fatalf("expected 2 channels, got %d", converted.Channels)
	}

	expected := []float64{-0.25, -0.25, 0.0, 0.0, 0.75, 0.75}
	for i, want := range expected {
		if converted.Data[i] != want {
			t.Fatalf("sample %d: expected %f, got %f", i, want, converted.Data[i])
		}
	}
}

func TestConvertChannelsRejectsUnsupportedConversion(t *testing.T) {
	t.Parallel()

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{0.1, 0.2},
	}

	if _, err := convert.ConvertChannels(buffer, convert.ChannelOptions{TargetChannels: 3}); err == nil {
		t.Fatal("expected unsupported conversion error")
	}
}
