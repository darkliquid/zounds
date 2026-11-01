package analysis_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/darkliquid/zounds/pkg/analysis"
	"github.com/darkliquid/zounds/pkg/core"
)

// Helper to write a RIFF chunk with 2-byte alignment padding.
func writeRIFFChunk(buf *bytes.Buffer, fourcc string, data []byte) {
	buf.WriteString(fourcc)
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	if len(data)%2 != 0 {
		buf.WriteByte(0)
	}
}

// Helper to write an IFF (AIFF) chunk with 2-byte alignment padding.
func writeIFFChunk(buf *bytes.Buffer, fourcc string, data []byte) {
	buf.WriteString(fourcc)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.Write(data)
	if len(data)%2 != 0 {
		buf.WriteByte(0)
	}
}

func writeMinimalWAVWithChunks(t *testing.T, path string, chunks ...func(buf *bytes.Buffer)) {
	t.Helper()

	var body bytes.Buffer
	// fmt chunk: PCM, 1 channel, 44100 Hz, 16 bits
	var fmtData bytes.Buffer
	_ = binary.Write(&fmtData, binary.LittleEndian, uint16(1))     // PCM
	_ = binary.Write(&fmtData, binary.LittleEndian, uint16(1))     // mono
	_ = binary.Write(&fmtData, binary.LittleEndian, uint32(44100)) // sample rate
	_ = binary.Write(&fmtData, binary.LittleEndian, uint32(88200)) // byte rate
	_ = binary.Write(&fmtData, binary.LittleEndian, uint16(2))     // block align
	_ = binary.Write(&fmtData, binary.LittleEndian, uint16(16))    // bits per sample
	writeRIFFChunk(&body, "fmt ", fmtData.Bytes())

	// Custom chunks
	for _, fn := range chunks {
		fn(&body)
	}

	// data chunk with a few dummy audio frames
	writeRIFFChunk(&body, "data", make([]byte, 100))

	// Outer RIFF header
	var fileBuf bytes.Buffer
	fileBuf.WriteString("RIFF")
	totalSize := uint32(4 + body.Len())
	_ = binary.Write(&fileBuf, binary.LittleEndian, totalSize)
	fileBuf.WriteString("WAVE")
	fileBuf.Write(body.Bytes())

	if err := os.WriteFile(path, fileBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("write wav fixture: %v", err)
	}
}

func writeMinimalAIFFWithChunks(t *testing.T, path string, chunks ...func(buf *bytes.Buffer)) {
	t.Helper()

	var body bytes.Buffer
	// COMM chunk: 1 channel, 100 frames, 16 bit, 44100 Hz (80-bit IEEE float)
	var commData bytes.Buffer
	_ = binary.Write(&commData, binary.BigEndian, int16(1))   // channels
	_ = binary.Write(&commData, binary.BigEndian, uint32(50)) // sample frames
	_ = binary.Write(&commData, binary.BigEndian, int16(16))  // bit depth
	// 44100 as 80-bit extended: 0x400E, 0xAC44000000000000
	commData.Write([]byte{0x40, 0x0E, 0xAC, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	writeIFFChunk(&body, "COMM", commData.Bytes())

	// Custom chunks
	for _, fn := range chunks {
		fn(&body)
	}

	// SSND chunk
	var ssndData bytes.Buffer
	_ = binary.Write(&ssndData, binary.BigEndian, uint32(0)) // offset
	_ = binary.Write(&ssndData, binary.BigEndian, uint32(0)) // block size
	ssndData.Write(make([]byte, 100))
	writeIFFChunk(&body, "SSND", ssndData.Bytes())

	// Outer FORM header
	var fileBuf bytes.Buffer
	fileBuf.WriteString("FORM")
	totalSize := uint32(4 + body.Len())
	_ = binary.Write(&fileBuf, binary.BigEndian, totalSize)
	fileBuf.WriteString("AIFF")
	fileBuf.Write(body.Bytes())

	if err := os.WriteFile(path, fileBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("write aiff fixture: %v", err)
	}
}

func TestParseWAVChunksSmpl(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sampler.wav")
	writeMinimalWAVWithChunks(t, path, func(buf *bytes.Buffer) {
		var smpl bytes.Buffer
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // manufacturer
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // product
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(22)) // sample period (ns)
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(60)) // MIDI unity note = 60 (C4)
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // pitch fraction
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // SMPTE format
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // SMPTE offset
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(1))  // Num sample loops = 1
		_ = binary.Write(&smpl, binary.LittleEndian, uint32(0))  // Sampler data size
		// 1 loop record (24 bytes)
		smpl.Write(make([]byte, 24))
		writeRIFFChunk(buf, "smpl", smpl.Bytes())
	})

	meta, err := analysis.ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}

	if meta.Values["midi_note"] != "60" {
		t.Errorf("expected midi_note '60', got %q", meta.Values["midi_note"])
	}
	if meta.Values["note_name"] != "C4" {
		t.Errorf("expected note_name 'C4', got %q", meta.Values["note_name"])
	}
	if meta.Values["is_loop"] != "true" || meta.Values["loop_class"] != "loop" {
		t.Errorf("expected loop_class 'loop', got is_loop=%q, loop_class=%q", meta.Values["is_loop"], meta.Values["loop_class"])
	}
}

func TestParseWAVChunksAcid(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "acid.wav")
	writeMinimalWAVWithChunks(t, path, func(buf *bytes.Buffer) {
		var acid bytes.Buffer
		_ = binary.Write(&acid, binary.LittleEndian, uint32(0x06))                // flags: loop (0x04) + root note valid (0x02)
		_ = binary.Write(&acid, binary.LittleEndian, uint16(69))                  // root note = 69 (A4)
		_ = binary.Write(&acid, binary.LittleEndian, uint16(0))                   // reserved
		_ = binary.Write(&acid, binary.LittleEndian, float32(0))                  // reserved
		_ = binary.Write(&acid, binary.LittleEndian, uint32(4))                   // num beats = 4
		_ = binary.Write(&acid, binary.LittleEndian, uint16(4))                   // meter num = 4
		_ = binary.Write(&acid, binary.LittleEndian, uint16(4))                   // meter den = 4
		_ = binary.Write(&acid, binary.LittleEndian, math.Float32bits(128.0))     // tempo = 128.0
		writeRIFFChunk(buf, "acid", acid.Bytes())
	})

	meta, err := analysis.ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}

	if meta.Values["bpm"] != "128.00" {
		t.Errorf("expected bpm '128.00', got %q", meta.Values["bpm"])
	}
	if meta.Values["num_beats"] != "4" {
		t.Errorf("expected num_beats '4', got %q", meta.Values["num_beats"])
	}
	if meta.Values["meter"] != "4/4" {
		t.Errorf("expected meter '4/4', got %q", meta.Values["meter"])
	}
	if meta.Values["midi_note"] != "69" || meta.Values["note_name"] != "A4" {
		t.Errorf("expected note A4 (69), got midi_note=%q note_name=%q", meta.Values["midi_note"], meta.Values["note_name"])
	}
	if meta.Values["loop_class"] != "loop" {
		t.Errorf("expected loop_class 'loop', got %q", meta.Values["loop_class"])
	}
}

func TestParseWAVChunksBext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "bext.wav")
	writeMinimalWAVWithChunks(t, path, func(buf *bytes.Buffer) {
		var bext [602]byte
		copy(bext[0:256], "Drum Loop Punchy")
		copy(bext[256:288], "Ableton Live")
		copy(bext[320:330], "2026-11-01")
		copy(bext[330:338], "12:30:00")
		// Version 1
		binary.LittleEndian.PutUint16(bext[346:348], 1)
		// Loudness: -1400 (-14.00 LUFS)
		l := int16(-1400)
		binary.LittleEndian.PutUint16(bext[412:414], uint16(l))
		// Max true peak: -100 (-1.00 dBTP)
		p := int16(-100)
		binary.LittleEndian.PutUint16(bext[416:418], uint16(p))

		writeRIFFChunk(buf, "bext", bext[:])
	})

	meta, err := analysis.ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}

	if meta.Values["description"] != "Drum Loop Punchy" {
		t.Errorf("expected description 'Drum Loop Punchy', got %q", meta.Values["description"])
	}
	if meta.Values["originator"] != "Ableton Live" {
		t.Errorf("expected originator 'Ableton Live', got %q", meta.Values["originator"])
	}
	if meta.Values["origination_date"] != "2026-11-01" {
		t.Errorf("expected date '2026-11-01', got %q", meta.Values["origination_date"])
	}
	if meta.Values["bext_loudness_lufs"] != "-14.00" {
		t.Errorf("expected loudness '-14.00', got %q", meta.Values["bext_loudness_lufs"])
	}
	if meta.Values["bext_true_peak_dbtp"] != "-1.00" {
		t.Errorf("expected true peak '-1.00', got %q", meta.Values["bext_true_peak_dbtp"])
	}
}

func TestParseWAVChunksListInfo(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "info.wav")
	writeMinimalWAVWithChunks(t, path, func(buf *bytes.Buffer) {
		var infoList bytes.Buffer
		infoList.WriteString("INFO")
		writeRIFFChunk(&infoList, "INAM", []byte("Sub Bass Hit\x00"))
		writeRIFFChunk(&infoList, "IART", []byte("Producer X\x00"))
		writeRIFFChunk(&infoList, "IGNR", []byte("Bass Music\x00"))
		writeRIFFChunk(&infoList, "IKEY", []byte("F#m\x00"))
		writeRIFFChunk(buf, "LIST", infoList.Bytes())
	})

	meta, err := analysis.ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}

	if meta.Values["title"] != "Sub Bass Hit" {
		t.Errorf("expected title 'Sub Bass Hit', got %q", meta.Values["title"])
	}
	if meta.Values["artist"] != "Producer X" {
		t.Errorf("expected artist 'Producer X', got %q", meta.Values["artist"])
	}
	if meta.Values["genre"] != "Bass Music" {
		t.Errorf("expected genre 'Bass Music', got %q", meta.Values["genre"])
	}
	if meta.Values["key"] != "F# minor" {
		t.Errorf("expected key 'F# minor', got %q", meta.Values["key"])
	}
	if meta.Values["tonic"] != "F#" || meta.Values["mode"] != "minor" {
		t.Errorf("expected tonic 'F#' mode 'minor', got tonic=%q mode=%q", meta.Values["tonic"], meta.Values["mode"])
	}
}

func TestParseAIFFChunksBasc(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "apple_loop.aif")
	writeMinimalAIFFWithChunks(t, path, func(buf *bytes.Buffer) {
		var basc bytes.Buffer
		_ = binary.Write(&basc, binary.BigEndian, uint32(1))  // version
		_ = binary.Write(&basc, binary.BigEndian, uint32(1))  // type: 1 = loop
		_ = binary.Write(&basc, binary.BigEndian, uint32(8))  // num beats = 8
		_ = binary.Write(&basc, binary.BigEndian, uint16(57)) // root note = 57 (A3)
		_ = binary.Write(&basc, binary.BigEndian, uint16(2))  // scale type: 2 = minor
		_ = binary.Write(&basc, binary.BigEndian, uint16(0))  // key
		_ = binary.Write(&basc, binary.BigEndian, uint16(4))  // meter num
		_ = binary.Write(&basc, binary.BigEndian, uint16(4))  // meter den
		writeIFFChunk(buf, "basc", basc.Bytes())
	})

	meta, err := analysis.ReadEmbeddedMetadata(path)
	if err != nil {
		t.Fatalf("ReadEmbeddedMetadata: %v", err)
	}

	if meta.Values["loop_class"] != "loop" || meta.Values["is_loop"] != "true" {
		t.Errorf("expected loop, got loop_class=%q is_loop=%q", meta.Values["loop_class"], meta.Values["is_loop"])
	}
	if meta.Values["num_beats"] != "8" {
		t.Errorf("expected num_beats '8', got %q", meta.Values["num_beats"])
	}
	if meta.Values["note_name"] != "A3" || meta.Values["midi_note"] != "57" {
		t.Errorf("expected A3 (57), got note_name=%q midi_note=%q", meta.Values["note_name"], meta.Values["midi_note"])
	}
	if meta.Values["key"] != "A minor" {
		t.Errorf("expected key 'A minor', got %q", meta.Values["key"])
	}
}

func TestNormalizeMusicalKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input     string
		wantKey   string
		wantTonic string
		wantMode  string
	}{
		{"C", "C major", "C", "major"},
		{"C major", "C major", "C", "major"},
		{"Cmaj", "C major", "C", "major"},
		{"Cm", "C minor", "C", "minor"},
		{"C min", "C minor", "C", "minor"},
		{"C minor", "C minor", "C", "minor"},
		{"F#m", "F# minor", "F#", "minor"},
		{"Gb", "Gb major", "Gb", "major"},
		{"8B", "C major", "C", "major"},
		{"8A", "A minor", "A", "minor"},
		{"11A", "F# minor", "F#", "minor"},
		{"4B", "Ab major", "Ab", "major"},
	}

	for _, tc := range cases {
		key, tonic, mode, ok := analysis.NormalizeMusicalKey(tc.input)
		if !ok {
			t.Errorf("failed to parse key %q", tc.input)
			continue
		}
		if key != tc.wantKey || tonic != tc.wantTonic || mode != tc.wantMode {
			t.Errorf("NormalizeMusicalKey(%q) = (%q, %q, %q), want (%q, %q, %q)", tc.input, key, tonic, mode, tc.wantKey, tc.wantTonic, tc.wantMode)
		}
	}
}

func TestMetadataAnalyzerPrioritizesEmbeddedMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "prior.wav")
	writeMinimalWAVWithChunks(t, path, func(buf *bytes.Buffer) {
		var acid bytes.Buffer
		_ = binary.Write(&acid, binary.LittleEndian, uint32(0x04))            // flags: loop (0x04)
		_ = binary.Write(&acid, binary.LittleEndian, uint16(60))              // root note = 60 (C4)
		_ = binary.Write(&acid, binary.LittleEndian, uint16(0))               // reserved
		_ = binary.Write(&acid, binary.LittleEndian, float32(0))              // reserved
		_ = binary.Write(&acid, binary.LittleEndian, uint32(8))               // num beats = 8
		_ = binary.Write(&acid, binary.LittleEndian, uint16(4))               // meter num = 4
		_ = binary.Write(&acid, binary.LittleEndian, uint16(4))               // meter den = 4
		_ = binary.Write(&acid, binary.LittleEndian, math.Float32bits(174.0)) // tempo = 174.0 BPM
		writeRIFFChunk(buf, "acid", acid.Bytes())

		var infoList bytes.Buffer
		infoList.WriteString("INFO")
		writeRIFFChunk(&infoList, "IKEY", []byte("11A\x00")) // F# minor in Camelot
		writeRIFFChunk(buf, "LIST", infoList.Bytes())
	})

	analyzer, err := analysis.NewMetadataAnalyzer(nil)
	if err != nil {
		t.Fatalf("NewMetadataAnalyzer: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	result, err := analyzer.Analyze(context.Background(), core.Sample{
		Path:      path,
		Extension: "wav",
		Format:    core.FormatWAV,
		SizeBytes: info.Size(),
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if result.Metrics["tempo_bpm"] != 174.0 || result.Metrics["tempo_confidence"] != 1.0 {
		t.Errorf("expected tempo_bpm 174.0 with confidence 1.0, got bpm=%f conf=%f", result.Metrics["tempo_bpm"], result.Metrics["tempo_confidence"])
	}
	if result.Attributes["key"] != "F# minor" || result.Attributes["tonic"] != "F#" || result.Attributes["mode"] != "minor" {
		t.Errorf("expected key 'F# minor', got key=%q tonic=%q mode=%q", result.Attributes["key"], result.Attributes["tonic"], result.Attributes["mode"])
	}
	if result.Metrics["key_confidence"] != 1.0 {
		t.Errorf("expected key_confidence 1.0, got %f", result.Metrics["key_confidence"])
	}
	if result.Attributes["loop_class"] != "loop" || result.Metrics["loop_confidence"] != 1.0 {
		t.Errorf("expected loop_class 'loop' with confidence 1.0, got class=%q conf=%f", result.Attributes["loop_class"], result.Metrics["loop_confidence"])
	}
	if result.Metrics["midi_note"] != 60 || result.Attributes["note_name"] != "C4" {
		t.Errorf("expected midi_note 60 / C4, got note=%f name=%q", result.Metrics["midi_note"], result.Attributes["note_name"])
	}
}
