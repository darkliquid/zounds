package audio

import (
	"errors"
	"fmt"
	"maps"
	"time"
)

type PCMBuffer struct {
	SampleRate int
	Channels   int
	BitDepth   int
	Data       []float64
	Metadata   map[string]string
}

func (b PCMBuffer) Frames() int {
	if b.Channels <= 0 {
		return 0
	}
	return len(b.Data) / b.Channels
}

func (b PCMBuffer) Duration() time.Duration {
	if b.SampleRate <= 0 || b.Channels <= 0 || len(b.Data) == 0 {
		return 0
	}

	seconds := float64(b.Frames()) / float64(b.SampleRate)
	return time.Duration(seconds * float64(time.Second))
}

func (b PCMBuffer) Clone() PCMBuffer {
	clone := PCMBuffer{
		SampleRate: b.SampleRate,
		Channels:   b.Channels,
		BitDepth:   b.BitDepth,
		Data:       append([]float64(nil), b.Data...),
	}

	if len(b.Metadata) > 0 {
		clone.Metadata = maps.Clone(b.Metadata)
	}

	return clone
}

func (b PCMBuffer) Validate() error {
	switch {
	case b.SampleRate <= 0:
		return errors.New("sample rate must be greater than zero")
	case b.Channels <= 0:
		return errors.New("channels must be greater than zero")
	case len(b.Data) == 0:
		return errors.New("buffer data is empty")
	case len(b.Data)%b.Channels != 0:
		return fmt.Errorf("buffer data length %d is not divisible by channel count %d", len(b.Data), b.Channels)
	}

	for i, sample := range b.Data {
		if sample < -1.0 || sample > 1.0 {
			return fmt.Errorf("sample at index %d is out of range [-1, 1]: %f", i, sample)
		}
	}

	return nil
}
