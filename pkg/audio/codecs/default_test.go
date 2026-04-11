package codecs_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestNewRegistryRegistersBuiltInCodecs(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("create codec registry: %v", err)
	}

	for _, format := range []core.AudioFormat{
		core.FormatWAV,
		core.FormatAIFF,
		core.FormatFLAC,
		core.FormatMP3,
		core.FormatOGG,
	} {
		if _, ok := registry.Decoder(format); !ok {
			t.Fatalf("missing decoder for %s", format)
		}
	}
}
