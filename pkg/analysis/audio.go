package analysis

import (
	"context"
	"fmt"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	"github.com/darkliquid/zounds/pkg/core"
)

type decodedSampleContextKey struct{}

type decodedSampleContextValue struct {
	path    string
	decoded zaudio.DecodeResult
}

func ContextWithDecodedSample(ctx context.Context, samplePath string, decoded zaudio.DecodeResult) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, decodedSampleContextKey{}, decodedSampleContextValue{
		path:    samplePath,
		decoded: decoded,
	})
}

func defaultRegistry(registry *zaudio.Registry) (*zaudio.Registry, error) {
	if registry != nil {
		return registry, nil
	}

	created, err := codecs.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create default codec registry: %w", err)
	}

	return created, nil
}

func decodeSample(ctx context.Context, registry *zaudio.Registry, sample core.Sample) (zaudio.DecodeResult, error) {
	if registry == nil {
		return zaudio.DecodeResult{}, fmt.Errorf("analyzer is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if value, ok := ctx.Value(decodedSampleContextKey{}).(decodedSampleContextValue); ok && value.path == sample.Path {
		return value.decoded, nil
	}

	decoded, err := zaudio.DecodeFile(ctx, registry, sample.Path)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode sample %q: %w", sample.Path, err)
	}

	return decoded, nil
}
