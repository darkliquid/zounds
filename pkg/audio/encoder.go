package audio

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/zounds/pkg/core"
)

type Encoder interface {
	Format() core.AudioFormat
	Encode(ctx context.Context, writer io.WriteSeeker, buffer PCMBuffer) error
}

func EncodeFile(ctx context.Context, registry *Registry, path string, buffer PCMBuffer) error {
	if registry == nil {
		return fmt.Errorf("encode %q: nil registry", path)
	}

	encoder, ok := registry.Encoder(core.DetectFormatFromExtension(path))
	if !ok {
		return fmt.Errorf("encode %q: no encoder registered for %s", path, core.DetectFormatFromExtension(path))
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	return encoder.Encode(ctx, file, buffer)
}
