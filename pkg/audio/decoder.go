package audio

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/zounds/pkg/core"
)

type StreamInfo struct {
	Format     core.AudioFormat
	SampleRate int
	Channels   int
	BitDepth   int
	Bitrate    int
	Frames     int
}

type DecodeResult struct {
	Buffer PCMBuffer
	Info   StreamInfo
}

type Decoder interface {
	Format() core.AudioFormat
	Decode(ctx context.Context, reader io.ReadSeeker) (DecodeResult, error)
}

func DecodeFile(ctx context.Context, registry *Registry, path string) (DecodeResult, error) {
	if registry == nil {
		return DecodeResult{}, fmt.Errorf("decode %q: nil registry", path)
	}

	decoder, ok := registry.Decoder(core.DetectFormatFromExtension(path))
	if !ok {
		return DecodeResult{}, fmt.Errorf("decode %q: no decoder registered for %s", path, core.DetectFormatFromExtension(path))
	}

	file, err := os.Open(path)
	if err != nil {
		return DecodeResult{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer file.Close()

	return decoder.Decode(ctx, file)
}
