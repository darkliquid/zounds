package audio_test

import (
	"context"
	"io"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

type fakeCodec struct {
	format core.AudioFormat
}

func (f fakeCodec) Format() core.AudioFormat { return f.format }
func (f fakeCodec) Decode(context.Context, io.ReadSeeker) (zaudio.DecodeResult, error) {
	return zaudio.DecodeResult{}, nil
}
func (f fakeCodec) Encode(context.Context, io.WriteSeeker, zaudio.PCMBuffer) error {
	return nil
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	t.Parallel()

	registry := zaudio.NewRegistry()
	if err := registry.RegisterDecoder(fakeCodec{format: core.FormatWAV}); err != nil {
		t.Fatalf("register decoder: %v", err)
	}
	if err := registry.RegisterEncoder(fakeCodec{format: core.FormatWAV}); err != nil {
		t.Fatalf("register encoder: %v", err)
	}

	if _, ok := registry.Decoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav decoder")
	}
	if _, ok := registry.Encoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav encoder")
	}

	formats := registry.Formats()
	if len(formats) != 1 || formats[0] != core.FormatWAV {
		t.Fatalf("unexpected registered formats: %v", formats)
	}
}
