package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

type PlaybackOptions struct {
	Registry      *Registry
	BufferSize    time.Duration
	DeviceFactory func(sampleRate, channels int, bufferSize time.Duration) (PlaybackDevice, error)
}

type Playback struct {
	registry      *Registry
	bufferSize    time.Duration
	deviceFactory func(sampleRate, channels int, bufferSize time.Duration) (PlaybackDevice, error)

	mu         sync.Mutex
	device     PlaybackDevice
	player     PlaybackPlayer
	sampleRate int
	channels   int
}

type PlaybackDevice interface {
	NewPlayer(io.Reader) PlaybackPlayer
	Resume() error
}

type PlaybackPlayer interface {
	Play()
	Pause()
	Close() error
	IsPlaying() bool
	SetVolume(volume float64)
	Volume() float64
}

func NewPlayback(opts PlaybackOptions) (*Playback, error) {
	registry := opts.Registry
	if registry == nil {
		registry = NewRegistry()
	}

	factory := opts.DeviceFactory
	if factory == nil {
		factory = newOtoDevice
	}

	return &Playback{
		registry:      registry,
		bufferSize:    opts.BufferSize,
		deviceFactory: factory,
	}, nil
}

func (p *Playback) PlayFile(ctx context.Context, path string) error {
	result, err := DecodeFile(ctx, p.registry, path)
	if err != nil {
		return err
	}

	return p.PlayBuffer(ctx, result.Buffer)
}

func (p *Playback) PlayBuffer(ctx context.Context, buffer PCMBuffer) error {
	if err := buffer.Validate(); err != nil {
		return fmt.Errorf("play buffer: %w", err)
	}
	if buffer.Channels < 1 || buffer.Channels > 2 {
		return fmt.Errorf("play buffer: unsupported channel count %d", buffer.Channels)
	}

	data, err := pcmToFloat32LE(buffer)
	if err != nil {
		return fmt.Errorf("play buffer: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureDevice(buffer.SampleRate, buffer.Channels); err != nil {
		return err
	}
	if p.player != nil {
		_ = p.player.Close()
		p.player = nil
	}

	reader := bytes.NewReader(data)
	player := p.device.NewPlayer(reader)
	p.player = player
	player.Play()

	if ctx != nil {
		go func(player PlaybackPlayer) {
			<-ctx.Done()
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.player == player {
				_ = p.player.Close()
				p.player = nil
			}
		}(player)
	}

	return nil
}

func (p *Playback) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.player == nil {
		return nil
	}
	err := p.player.Close()
	p.player = nil
	return err
}

func (p *Playback) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.player != nil && p.player.IsPlaying()
}

func (p *Playback) SetVolume(volume float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.player != nil {
		p.player.SetVolume(volume)
	}
}

func (p *Playback) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.player == nil {
		return 0
	}
	return p.player.Volume()
}

func (p *Playback) ensureDevice(sampleRate, channels int) error {
	if p.device != nil {
		if p.sampleRate != sampleRate || p.channels != channels {
			return fmt.Errorf("playback device already initialized for %d Hz/%d channels, cannot play %d Hz/%d channels", p.sampleRate, p.channels, sampleRate, channels)
		}
		return nil
	}

	device, err := p.deviceFactory(sampleRate, channels, p.bufferSize)
	if err != nil {
		return fmt.Errorf("create playback device: %w", err)
	}
	if err := device.Resume(); err != nil {
		return fmt.Errorf("resume playback device: %w", err)
	}

	p.device = device
	p.sampleRate = sampleRate
	p.channels = channels
	return nil
}

func pcmToFloat32LE(buffer PCMBuffer) ([]byte, error) {
	if err := buffer.Validate(); err != nil {
		return nil, err
	}

	data := make([]byte, len(buffer.Data)*4)
	for i, sample := range buffer.Data {
		clamped := math.Max(-1, math.Min(1, sample))
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(float32(clamped)))
	}
	return data, nil
}

type otoDevice struct {
	context *oto.Context
}

func newOtoDevice(sampleRate, channels int, bufferSize time.Duration) (PlaybackDevice, error) {
	context, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: channels,
		Format:       oto.FormatFloat32LE,
		BufferSize:   bufferSize,
	})
	if err != nil {
		return nil, err
	}
	<-ready
	return &otoDevice{context: context}, nil
}

func (d *otoDevice) NewPlayer(reader io.Reader) PlaybackPlayer {
	return d.context.NewPlayer(reader)
}

func (d *otoDevice) Resume() error {
	return d.context.Resume()
}
