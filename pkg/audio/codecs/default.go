package codecs

import (
	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/aiff"
	"github.com/darkliquid/zounds/pkg/audio/flac"
	"github.com/darkliquid/zounds/pkg/audio/mp3"
	"github.com/darkliquid/zounds/pkg/audio/ogg"
	"github.com/darkliquid/zounds/pkg/audio/wav"
)

func NewRegistry() (*zaudio.Registry, error) {
	registry := zaudio.NewRegistry()

	for _, register := range []func(*zaudio.Registry) error{
		wav.Register,
		aiff.Register,
		flac.Register,
		mp3.Register,
		ogg.Register,
	} {
		if err := register(registry); err != nil {
			return nil, err
		}
	}

	return registry, nil
}
