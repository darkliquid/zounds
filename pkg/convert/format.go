package convert

import (
	"context"
	"fmt"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func SupportedTargetFormats(registry *zaudio.Registry) []core.AudioFormat {
	if registry == nil {
		return nil
	}

	formats := registry.Formats()
	targets := make([]core.AudioFormat, 0, len(formats))
	for _, format := range formats {
		if _, ok := registry.Encoder(format); ok {
			targets = append(targets, format)
		}
	}

	return targets
}

func TranscodeFile(ctx context.Context, registry *zaudio.Registry, sourcePath, targetPath string) error {
	if registry == nil {
		return fmt.Errorf("transcode %q -> %q: nil registry", sourcePath, targetPath)
	}

	targetFormat := core.DetectFormatFromExtension(targetPath)
	if _, ok := registry.Encoder(targetFormat); !ok {
		return fmt.Errorf("transcode %q -> %q: no encoder registered for %s", sourcePath, targetPath, targetFormat)
	}

	result, err := zaudio.DecodeFile(ctx, registry, sourcePath)
	if err != nil {
		return fmt.Errorf("transcode %q -> %q: %w", sourcePath, targetPath, err)
	}

	if err := zaudio.EncodeFile(ctx, registry, targetPath, result.Buffer); err != nil {
		return fmt.Errorf("transcode %q -> %q: %w", sourcePath, targetPath, err)
	}

	return nil
}
