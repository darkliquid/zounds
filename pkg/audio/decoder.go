package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/zounds/pkg/core"
)

var ErrUnsupportedFormat = errors.New("unsupported audio format")

type DecodeError struct {
	Path   string
	Format core.AudioFormat
	Err    error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("decode %q (%s): %v", e.Path, e.Format, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

func IsDecodeError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var target *DecodeError
	return errors.As(err, &target)
}

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
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return DecodeResult{}, err
		}
	}
	if registry == nil {
		return DecodeResult{}, fmt.Errorf("decode %q: nil registry", path)
	}

	format := core.DetectFormatFromExtension(path)
	decoder, ok := registry.Decoder(format)
	if !ok {
		return DecodeResult{}, &DecodeError{
			Path:   path,
			Format: format,
			Err:    fmt.Errorf("%w: no decoder registered for %s", ErrUnsupportedFormat, format),
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return DecodeResult{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	decoded, err := decoder.Decode(ctx, file)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return DecodeResult{}, err
		}
		return DecodeResult{}, &DecodeError{
			Path:   path,
			Format: format,
			Err:    err,
		}
	}
	return decoded, nil
}
