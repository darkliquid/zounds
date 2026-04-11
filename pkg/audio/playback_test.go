package audio_test

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestPlaybackPlayBufferUsesFloat32PCM(t *testing.T) {
	t.Parallel()

	var fake *fakeDevice
	playback, err := zaudio.NewPlayback(zaudio.PlaybackOptions{
		DeviceFactory: func(sampleRate, channels int, bufferSize time.Duration) (zaudio.PlaybackDevice, error) {
			fake = &fakeDevice{}
			return fake, nil
		},
	})
	if err != nil {
		t.Fatalf("create playback: %v", err)
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-1, -0.5, 0, 0.5, 1},
	}
	if err := playback.PlayBuffer(context.Background(), buffer); err != nil {
		t.Fatalf("play buffer: %v", err)
	}

	if !fake.player.playing {
		t.Fatalf("expected fake player to be playing")
	}
	if len(fake.player.data) != len(buffer.Data)*4 {
		t.Fatalf("unexpected byte length: %d", len(fake.player.data))
	}

	first := math.Float32frombits(binary.LittleEndian.Uint32(fake.player.data[0:4]))
	last := math.Float32frombits(binary.LittleEndian.Uint32(fake.player.data[len(fake.player.data)-4:]))
	if first != -1 || last != 1 {
		t.Fatalf("unexpected decoded float32 endpoints: %f %f", first, last)
	}
}

func TestPlaybackPlayFileDecodesAndPlaysWAV(t *testing.T) {
	t.Parallel()

	registry := zaudio.NewRegistry()
	if err := registry.RegisterDecoder(fakeDecoder{}); err != nil {
		t.Fatalf("register fake decoder: %v", err)
	}

	var fake *fakeDevice
	playback, err := zaudio.NewPlayback(zaudio.PlaybackOptions{
		Registry: registry,
		DeviceFactory: func(sampleRate, channels int, bufferSize time.Duration) (zaudio.PlaybackDevice, error) {
			fake = &fakeDevice{}
			return fake, nil
		},
	})
	if err != nil {
		t.Fatalf("create playback: %v", err)
	}

	path := filepath.Join(t.TempDir(), "fixture.wav")
	if err := os.WriteFile(path, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fake file: %v", err)
	}

	if err := playback.PlayFile(context.Background(), path); err != nil {
		t.Fatalf("play file: %v", err)
	}
	if fake == nil || !fake.player.playing {
		t.Fatalf("expected playback to start")
	}
}

type fakeDevice struct {
	player *fakePlayer
}

func (d *fakeDevice) NewPlayer(r io.Reader) zaudio.PlaybackPlayer {
	data, _ := io.ReadAll(r)
	d.player = &fakePlayer{data: data}
	return d.player
}

func (d *fakeDevice) Resume() error { return nil }

type fakeDecoder struct{}

func (fakeDecoder) Format() core.AudioFormat {
	return core.FormatWAV
}

func (fakeDecoder) Decode(context.Context, io.ReadSeeker) (zaudio.DecodeResult, error) {
	return zaudio.DecodeResult{
		Buffer: zaudio.PCMBuffer{
			SampleRate: 44100,
			Channels:   1,
			BitDepth:   16,
			Data:       []float64{0, 0.25, -0.25, 0.5},
		},
	}, nil
}

type fakePlayer struct {
	data    []byte
	playing bool
	volume  float64
}

func (p *fakePlayer) Play()                    { p.playing = true }
func (p *fakePlayer) Pause()                   { p.playing = false }
func (p *fakePlayer) Close() error             { p.playing = false; return nil }
func (p *fakePlayer) IsPlaying() bool          { return p.playing }
func (p *fakePlayer) SetVolume(volume float64) { p.volume = volume }
func (p *fakePlayer) Volume() float64          { return p.volume }
