package wav

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	rawaudio "github.com/go-audio/audio"
	gowav "github.com/go-audio/wav"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const (
	waveFormatPCM        uint16 = 0x0001
	waveFormatIEEEFloat  uint16 = 0x0003
	waveFormatExtensible uint16 = 0xFFFE
)

var ksDataFormatSubtypeTail = [14]byte{
	0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71,
}

type wavFormatHeader struct {
	audioFormat   uint16
	effectiveFmt  uint16
	channels      uint16
	sampleRate    uint32
	blockAlign    uint16
	bitsPerSample uint16
	dataOffset    int64
	dataSize      uint32
}

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatWAV
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	hdr, err := parseWAVHeader(reader)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: %w", err)
	}

	if hdr.effectiveFmt == waveFormatIEEEFloat {
		return decodeIEEEFloatWAV(reader, hdr)
	}
	if hdr.effectiveFmt != waveFormatPCM {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: unsupported wave format tag 0x%04x", hdr.effectiveFmt)
	}

	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav rewind: %w", err)
	}

	decoder := gowav.NewDecoder(reader)
	if !decoder.IsValidFile() {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: invalid file")
	}

	intBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav pcm buffer: %w", err)
	}

	var buffer zaudio.PCMBuffer
	if intBuffer.SourceBitDepth == 8 {
		buffer, err = zaudio.Unsigned8IntBufferToPCM(intBuffer)
	} else {
		buffer, err = zaudio.IntBufferToPCM(intBuffer, intBuffer.SourceBitDepth)
	}
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav pcm conversion: %w", err)
	}
	buffer.Metadata = map[string]string{"codec": "wav"}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatWAV,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func parseWAVHeader(reader io.ReadSeeker) (wavFormatHeader, error) {
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return wavFormatHeader{}, err
	}

	var riffHdr [12]byte
	if _, err := io.ReadFull(reader, riffHdr[:]); err != nil {
		return wavFormatHeader{}, fmt.Errorf("invalid RIFF header: %w", err)
	}
	if string(riffHdr[0:4]) != "RIFF" || string(riffHdr[8:12]) != "WAVE" {
		return wavFormatHeader{}, errors.New("not a RIFF/WAVE stream")
	}

	var (
		hdr      wavFormatHeader
		foundFmt bool
		foundDat bool
	)
	for !foundFmt || !foundDat {
		var chunkHdr [8]byte
		if _, err := io.ReadFull(reader, chunkHdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return wavFormatHeader{}, fmt.Errorf("read RIFF chunk header: %w", err)
		}
		chunkID := string(chunkHdr[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHdr[4:8])
		chunkStart, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return wavFormatHeader{}, err
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return wavFormatHeader{}, fmt.Errorf("fmt chunk too small (%d bytes)", chunkSize)
			}
			fmtBytes := make([]byte, chunkSize)
			if _, err := io.ReadFull(reader, fmtBytes); err != nil {
				return wavFormatHeader{}, fmt.Errorf("read fmt chunk: %w", err)
			}
			hdr.audioFormat = binary.LittleEndian.Uint16(fmtBytes[0:2])
			hdr.effectiveFmt = hdr.audioFormat
			hdr.channels = binary.LittleEndian.Uint16(fmtBytes[2:4])
			hdr.sampleRate = binary.LittleEndian.Uint32(fmtBytes[4:8])
			hdr.blockAlign = binary.LittleEndian.Uint16(fmtBytes[12:14])
			hdr.bitsPerSample = binary.LittleEndian.Uint16(fmtBytes[14:16])

			if hdr.audioFormat == waveFormatExtensible {
				if len(fmtBytes) < 40 {
					return wavFormatHeader{}, fmt.Errorf("extensible fmt chunk too small (%d bytes)", len(fmtBytes))
				}
				var tail [14]byte
				copy(tail[:], fmtBytes[26:40])
				if tail != ksDataFormatSubtypeTail {
					return wavFormatHeader{}, errors.New("unsupported WAVE_FORMAT_EXTENSIBLE subformat GUID")
				}
				hdr.effectiveFmt = binary.LittleEndian.Uint16(fmtBytes[24:26])
			}
			foundFmt = true

		case "data":
			hdr.dataOffset = chunkStart
			hdr.dataSize = chunkSize
			foundDat = true
		}

		nextOffset := chunkStart + int64(chunkSize)
		if chunkSize%2 == 1 {
			nextOffset++
		}
		if !foundFmt || !foundDat {
			if _, err := reader.Seek(nextOffset, io.SeekStart); err != nil {
				break
			}
		}
	}

	if !foundFmt || !foundDat {
		return wavFormatHeader{}, errors.New("missing fmt or data chunk")
	}
	if hdr.channels == 0 || hdr.sampleRate == 0 || hdr.bitsPerSample < 8 {
		return wavFormatHeader{}, errors.New("invalid wav stream parameters")
	}

	return hdr, nil
}

func decodeIEEEFloatWAV(reader io.ReadSeeker, hdr wavFormatHeader) (zaudio.DecodeResult, error) {
	if hdr.bitsPerSample != 32 && hdr.bitsPerSample != 64 {
		return zaudio.DecodeResult{}, fmt.Errorf("decode ieee float wav: unsupported bit depth %d", hdr.bitsPerSample)
	}
	if _, err := reader.Seek(hdr.dataOffset, io.SeekStart); err != nil {
		return zaudio.DecodeResult{}, err
	}

	bytesPerSample := int(hdr.bitsPerSample / 8)
	frameBytes := bytesPerSample * int(hdr.channels)
	numFrames := int(hdr.dataSize) / frameBytes
	if numFrames <= 0 {
		return zaudio.DecodeResult{}, errors.New("decode ieee float wav: empty data chunk")
	}

	raw := make([]byte, numFrames*frameBytes)
	n, err := io.ReadFull(reader, raw)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return zaudio.DecodeResult{}, fmt.Errorf("read ieee float wav data: %w", err)
	}
	usableFrames := n / frameBytes
	if usableFrames <= 0 {
		return zaudio.DecodeResult{}, errors.New("decode ieee float wav: truncated data chunk")
	}
	raw = raw[:usableFrames*frameBytes]

	totalSamples := usableFrames * int(hdr.channels)
	data := make([]float64, totalSamples)
	for i := range totalSamples {
		offset := i * bytesPerSample
		var v float64
		if bytesPerSample == 4 {
			v = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset : offset+4])))
		} else {
			v = math.Float64frombits(binary.LittleEndian.Uint64(raw[offset : offset+8]))
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return zaudio.DecodeResult{}, fmt.Errorf("decode ieee float wav: non-finite sample at index %d", i)
		}
		if v < -1.0 {
			v = -1.0
		} else if v > 1.0 {
			v = 1.0
		}
		data[i] = v
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: int(hdr.sampleRate),
		Channels:   int(hdr.channels),
		BitDepth:   int(hdr.bitsPerSample),
		Data:       data,
		Metadata:   map[string]string{"codec": "wav"},
	}
	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatWAV,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func (Codec) Encode(ctx context.Context, writer io.WriteSeeker, buffer zaudio.PCMBuffer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := buffer.Validate(); err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}

	var (
		intBuffer *rawaudio.IntBuffer
		err       error
	)
	if buffer.BitDepth == 8 {
		intBuffer, err = zaudio.PCMToUnsigned8IntBuffer(buffer)
	} else {
		intBuffer, err = zaudio.PCMToIntBuffer(buffer)
	}
	if err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}

	encoder := gowav.NewEncoder(writer, buffer.SampleRate, intBuffer.SourceBitDepth, buffer.Channels, 1)
	if err := encoder.Write(intBuffer); err != nil {
		return fmt.Errorf("encode wav buffer: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finalize wav file: %w", err)
	}

	return nil
}

func Register(registry *zaudio.Registry) error {
	codec := New()
	if err := registry.RegisterDecoder(codec); err != nil {
		return err
	}
	if err := registry.RegisterEncoder(codec); err != nil {
		return err
	}
	return nil
}
