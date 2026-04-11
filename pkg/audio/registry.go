package audio

import (
	"fmt"
	"sort"
	"sync"

	"github.com/darkliquid/zounds/pkg/core"
)

type Registry struct {
	mu       sync.RWMutex
	decoders map[core.AudioFormat]Decoder
	encoders map[core.AudioFormat]Encoder
}

func NewRegistry() *Registry {
	return &Registry{
		decoders: make(map[core.AudioFormat]Decoder),
		encoders: make(map[core.AudioFormat]Encoder),
	}
}

func (r *Registry) RegisterDecoder(decoder Decoder) error {
	if decoder == nil {
		return fmt.Errorf("register decoder: nil decoder")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.decoders[decoder.Format()] = decoder
	return nil
}

func (r *Registry) RegisterEncoder(encoder Encoder) error {
	if encoder == nil {
		return fmt.Errorf("register encoder: nil encoder")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.encoders[encoder.Format()] = encoder
	return nil
}

func (r *Registry) Decoder(format core.AudioFormat) (Decoder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	decoder, ok := r.decoders[format]
	return decoder, ok
}

func (r *Registry) Encoder(format core.AudioFormat) (Encoder, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	encoder, ok := r.encoders[format]
	return encoder, ok
}

func (r *Registry) Formats() []core.AudioFormat {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[core.AudioFormat]struct{}, len(r.decoders)+len(r.encoders))
	for format := range r.decoders {
		seen[format] = struct{}{}
	}
	for format := range r.encoders {
		seen[format] = struct{}{}
	}

	formats := make([]core.AudioFormat, 0, len(seen))
	for format := range seen {
		formats = append(formats, format)
	}

	sort.Slice(formats, func(i, j int) bool {
		return formats[i] < formats[j]
	})

	return formats
}
