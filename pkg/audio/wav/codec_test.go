package wav_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	zwav "github.com/darkliquid/zounds/pkg/audio/wav"
	"github.com/darkliquid/zounds/pkg/core"
)

func TestWAVCodecRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	buffer := zaudio.PCMBuffer{
		SampleRate: 44100,
		Channels:   1,
		BitDepth:   16,
		Data:       []float64{-1.0, -0.5, 0.0, 0.5, 0.999},
	}

	filePath := filepath.Join(t.TempDir(), "roundtrip.wav")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create temp wav file: %v", err)
	}

	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode wav: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp wav file: %v", err)
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open temp wav file: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode wav: %v", err)
	}

	if decoded.Info.Format != core.FormatWAV {
		t.Fatalf("unexpected format: %s", decoded.Info.Format)
	}
	if decoded.Info.SampleRate != buffer.SampleRate {
		t.Fatalf("unexpected sample rate: %d", decoded.Info.SampleRate)
	}
	if decoded.Info.Channels != buffer.Channels {
		t.Fatalf("unexpected channels: %d", decoded.Info.Channels)
	}
	if decoded.Buffer.Frames() != len(buffer.Data) {
		t.Fatalf("unexpected frame count: %d", decoded.Buffer.Frames())
	}
}

func TestRegisterAddsWAVCodecToRegistry(t *testing.T) {
	t.Parallel()

	registry := zaudio.NewRegistry()
	if err := zwav.Register(registry); err != nil {
		t.Fatalf("register wav codec: %v", err)
	}

	if _, ok := registry.Decoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav decoder")
	}
	if _, ok := registry.Encoder(core.FormatWAV); !ok {
		t.Fatalf("expected wav encoder")
	}
}

func TestWAVCodec8BitUnsignedRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	want := []float64{-1.0, -0.5, 0.0, 0.5, 127.0 / 128.0}
	buffer := zaudio.PCMBuffer{
		SampleRate: 22050,
		Channels:   1,
		BitDepth:   8,
		Data:       want,
	}

	filePath := filepath.Join(t.TempDir(), "eightbit.wav")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create 8-bit wav: %v", err)
	}
	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode 8-bit wav: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close 8-bit wav: %v", err)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read raw 8-bit wav: %v", err)
	}
	dataIdx := bytes.Index(raw, []byte("data"))
	if dataIdx < 0 || dataIdx+8+len(want) > len(raw) {
		t.Fatalf("data chunk missing in 8-bit wav")
	}
	// Sample index 2 is 0.0 -> must be 0x80 (128) in unsigned RIFF WAVE 8-bit PCM.
	if raw[dataIdx+8+2] != 128 {
		t.Fatalf("expected 128 silence byte in 8-bit WAV, got %d", raw[dataIdx+8+2])
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open 8-bit wav: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode 8-bit wav: %v", err)
	}
	if err := decoded.Buffer.Validate(); err != nil {
		t.Fatalf("validate decoded 8-bit wav: %v", err)
	}
	for i := range want {
		if math.Abs(decoded.Buffer.Data[i]-want[i]) > 0.02 {
			t.Fatalf("sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
		}
	}
}

func TestWAVCodecDecodesIEEEFloat32AndFloat64AndExtensible(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	samples := []float64{-1.0, -0.25, 0.0, 0.25, 0.875, 1.05} // includes slight DAW overshoot 1.05 -> clamped to 1.0
	want := []float64{-1.0, -0.25, 0.0, 0.25, 0.875, 1.0}

	t.Run("float32_format3", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 3, 32, 48000, 2, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode float32 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate float32 wav: %v", err)
		}
		if decoded.Info.SampleRate != 48000 || decoded.Info.Channels != 2 || decoded.Info.BitDepth != 32 {
			t.Fatalf("unexpected stream info: %+v", decoded.Info)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-6 {
				t.Fatalf("float32 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})

	t.Run("float64_format3", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 3, 64, 96000, 1, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode float64 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate float64 wav: %v", err)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-12 {
				t.Fatalf("float64 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})

	t.Run("float32_extensible_fffe", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 0xFFFE, 32, 44100, 2, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode extensible float32 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate extensible float32 wav: %v", err)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-6 {
				t.Fatalf("extensible float32 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})
}

func buildFloatWAVFixture(t *testing.T, formatTag uint16, bitsPerSample uint16, sampleRate uint32, channels uint16, samples []float64) []byte {
	t.Helper()

	bytesPerSample := int(bitsPerSample / 8)
	dataPayload := new(bytes.Buffer)
	for _, s := range samples {
		switch bitsPerSample {
		case 32:
			_ = binary.Write(dataPayload, binary.LittleEndian, float32(s))
		case 64:
			_ = binary.Write(dataPayload, binary.LittleEndian, s)
		default:
			t.Fatalf("unsupported test bitsPerSample %d", bitsPerSample)
		}
	}

	fmtPayload := new(bytes.Buffer)
	blockAlign := channels * uint16(bytesPerSample)
	byteRate := sampleRate * uint32(blockAlign)
	_ = binary.Write(fmtPayload, binary.LittleEndian, formatTag)
	_ = binary.Write(fmtPayload, binary.LittleEndian, channels)
	_ = binary.Write(fmtPayload, binary.LittleEndian, sampleRate)
	_ = binary.Write(fmtPayload, binary.LittleEndian, byteRate)
	_ = binary.Write(fmtPayload, binary.LittleEndian, blockAlign)
	_ = binary.Write(fmtPayload, binary.LittleEndian, bitsPerSample)
	if formatTag == 0xFFFE {
		_ = binary.Write(fmtPayload, binary.LittleEndian, uint16(22))    // cbSize
		_ = binary.Write(fmtPayload, binary.LittleEndian, bitsPerSample) // wValidBitsPerSample
		_ = binary.Write(fmtPayload, binary.LittleEndian, uint32(0x3))   // dwChannelMask
		subFormatIEEEFloat := [16]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
		_, _ = fmtPayload.Write(subFormatIEEEFloat[:])
	}

	out := new(bytes.Buffer)
	_, _ = out.WriteString("RIFF")
	riffSize := uint32(4 + (8 + fmtPayload.Len()) + (8 + dataPayload.Len()))
	_ = binary.Write(out, binary.LittleEndian, riffSize)
	_, _ = out.WriteString("WAVE")
	_, _ = out.WriteString("fmt ")
	_ = binary.Write(out, binary.LittleEndian, uint32(fmtPayload.Len()))
	_, _ = out.Write(fmtPayload.Bytes())
	_, _ = out.WriteString("data")
	_ = binary.Write(out, binary.LittleEndian, uint32(dataPayload.Len()))
	_, _ = out.Write(dataPayload.Bytes())
	return out.Bytes()
}
