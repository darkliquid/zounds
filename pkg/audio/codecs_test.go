package audio_test

import (
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zaiff "github.com/darkliquid/zounds/pkg/audio/aiff"
	zflac "github.com/darkliquid/zounds/pkg/audio/flac"
	zmp3 "github.com/darkliquid/zounds/pkg/audio/mp3"
	zogg "github.com/darkliquid/zounds/pkg/audio/ogg"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestCompressedCodecsRegister(t *testing.T) {
	t.Parallel()

	registry := zaudio.NewRegistry()
	for _, register := range []func(*zaudio.Registry) error{
		zaiff.Register,
		zflac.Register,
		zmp3.Register,
		zogg.Register,
	} {
		if err := register(registry); err != nil {
			t.Fatalf("register codec: %v", err)
		}
	}

	for _, format := range []core.AudioFormat{core.FormatAIFF, core.FormatFLAC, core.FormatMP3, core.FormatOGG} {
		if _, ok := registry.Decoder(format); !ok {
			t.Fatalf("expected decoder for %s", format)
		}
	}
	if _, ok := registry.Encoder(core.FormatAIFF); !ok {
		t.Fatalf("expected encoder for %s", core.FormatAIFF)
	}
}
