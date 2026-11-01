package analysis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/dhowden/tag"
)

// ReadEmbeddedMetadata extracts metadata from standard tags (ID3, Vorbis, etc.)
// as well as WAV-specific chunks (smpl, acid, bext, LIST/INFO, id3 ) and AIFF-specific
// chunks (basc, ID3 , text chunks).
func ReadEmbeddedMetadata(path string) (EmbeddedMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return EmbeddedMetadata{}, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	values := map[string]string{}
	var (
		format     string
		fileType   string
		hasPicture bool
	)

	// 1. Try standard tag parsing via dhowden/tag
	if metadata, err := tag.ReadFrom(file); err == nil {
		format = string(metadata.Format())
		fileType = string(metadata.FileType())
		hasPicture = metadata.Picture() != nil

		trackNumber, trackTotal := metadata.Track()
		discNumber, discTotal := metadata.Disc()

		addMetadataValue(values, "title", metadata.Title())
		addMetadataValue(values, "album", metadata.Album())
		addMetadataValue(values, "artist", metadata.Artist())
		addMetadataValue(values, "album_artist", metadata.AlbumArtist())
		addMetadataValue(values, "composer", metadata.Composer())
		addMetadataValue(values, "genre", metadata.Genre())
		addMetadataValue(values, "comment", metadata.Comment())
		addMetadataValue(values, "lyrics", metadata.Lyrics())
		if metadata.Year() > 0 {
			values["year"] = fmt.Sprintf("%d", metadata.Year())
		}
		if trackNumber > 0 {
			values["track"] = fmt.Sprintf("%d", trackNumber)
		}
		if trackTotal > 0 {
			values["track_total"] = fmt.Sprintf("%d", trackTotal)
		}
		if discNumber > 0 {
			values["disc"] = fmt.Sprintf("%d", discNumber)
		}
		if discTotal > 0 {
			values["disc_total"] = fmt.Sprintf("%d", discTotal)
		}

		// Extract raw tags (e.g. TBPM, TKEY, Vorbis BPM/KEY)
		extractRawTags(metadata.Raw(), values)
	} else if !errors.Is(err, tag.ErrNoTagsFound) {
		// Non-fatal: continue to custom container chunk parsing
		_ = err
	}

	// 2. Container chunk parsing for WAV (RIFF) or AIFF (IFF)
	if _, err := file.Seek(0, io.SeekStart); err == nil {
		var magic [4]byte
		if _, err := io.ReadFull(file, magic[:]); err == nil {
			switch string(magic[:]) {
			case "RIFF":
				_ = parseWAVChunks(file, values)
			case "FORM":
				_ = parseAIFFChunks(file, values)
			}
		}
	}

	// 3. Post-process and normalize key if found
	if rawKey, ok := values["key"]; ok && rawKey != "" {
		if normKey, tonic, mode, ok := NormalizeMusicalKey(rawKey); ok {
			values["key"] = normKey
			values["tonic"] = tonic
			values["mode"] = mode
		}
	}

	return EmbeddedMetadata{
		Format:     format,
		FileType:   fileType,
		Values:     values,
		HasPicture: hasPicture,
	}, nil
}

func parseWAVChunks(r io.ReadSeeker, values map[string]string) error {
	// RIFF header: 4 bytes "RIFF", 4 bytes size, 4 bytes "WAVE"
	if _, err := r.Seek(4, io.SeekStart); err != nil {
		return err
	}
	var riffSize uint32
	if err := binary.Read(r, binary.LittleEndian, &riffSize); err != nil {
		return err
	}
	var waveMagic [4]byte
	if _, err := io.ReadFull(r, waveMagic[:]); err != nil || string(waveMagic[:]) != "WAVE" {
		return err
	}

	// Read chunks
	for {
		var chunkID [4]byte
		var chunkSize uint32
		if _, err := io.ReadFull(r, chunkID[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			break
		}

		id := string(chunkID[:])
		switch id {
		case "smpl":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseSmplChunk(data, values)
			}
		case "acid":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseAcidChunk(data, values)
			}
		case "bext":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseBextChunk(data, values)
			}
		case "LIST":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseListChunk(data, values)
			}
		case "id3 ", "ID3 ":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseID3v2Data(data, values)
			}
		default:
			// Skip chunk data
			if _, err := r.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return err
			}
		}

		// RIFF chunks are padded to 2-byte boundary
		if chunkSize%2 != 0 {
			if _, err := r.Seek(1, io.SeekCurrent); err != nil {
				break
			}
		}
	}

	return nil
}

func parseAIFFChunks(r io.ReadSeeker, values map[string]string) error {
	// FORM header: 4 bytes "FORM", 4 bytes size, 4 bytes "AIFF" / "AIFC"
	if _, err := r.Seek(4, io.SeekStart); err != nil {
		return err
	}
	var formSize uint32
	if err := binary.Read(r, binary.BigEndian, &formSize); err != nil {
		return err
	}
	var aiffMagic [4]byte
	if _, err := io.ReadFull(r, aiffMagic[:]); err != nil {
		return err
	}
	magicStr := string(aiffMagic[:])
	if magicStr != "AIFF" && magicStr != "AIFC" {
		return nil
	}

	for {
		var chunkID [4]byte
		var chunkSize uint32
		if _, err := io.ReadFull(r, chunkID[:]); err != nil {
			break
		}
		if err := binary.Read(r, binary.BigEndian, &chunkSize); err != nil {
			break
		}

		id := string(chunkID[:])
		switch id {
		case "basc":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseBascChunk(data, values)
			}
		case "ID3 ", "id3 ":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				parseID3v2Data(data, values)
			}
		case "NAME":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				addMetadataValue(values, "title", cleanString(string(data)))
			}
		case "AUTH":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				addMetadataValue(values, "artist", cleanString(string(data)))
			}
		case "(c) ":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				addMetadataValue(values, "copyright", cleanString(string(data)))
			}
		case "ANNO":
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, data); err == nil {
				addMetadataValue(values, "comment", cleanString(string(data)))
			}
		default:
			if _, err := r.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return err
			}
		}

		// IFF chunks are padded to 2-byte boundary
		if chunkSize%2 != 0 {
			if _, err := r.Seek(1, io.SeekCurrent); err != nil {
				break
			}
		}
	}

	return nil
}

func parseSmplChunk(data []byte, values map[string]string) {
	if len(data) < 36 {
		return
	}
	midiNote := binary.LittleEndian.Uint32(data[12:16])
	pitchFraction := binary.LittleEndian.Uint32(data[16:20])
	numLoops := binary.LittleEndian.Uint32(data[28:32])

	if midiNote <= 127 {
		values["midi_note"] = fmt.Sprintf("%d", midiNote)
		if name := MidiNoteToName(int(midiNote)); name != "" {
			values["note_name"] = name
		}
	}
	if pitchFraction > 0 {
		cents := float64(pitchFraction) / 4294967296.0 * 100.0
		values["fine_tune_cents"] = fmt.Sprintf("%.2f", cents)
	}
	if numLoops > 0 {
		values["is_loop"] = "true"
		values["loop_class"] = "loop"
		values["num_loops"] = fmt.Sprintf("%d", numLoops)
	}
}

func parseAcidChunk(data []byte, values map[string]string) {
	if len(data) < 24 {
		return
	}
	flags := binary.LittleEndian.Uint32(data[0:4])
	rootNote := binary.LittleEndian.Uint16(data[4:6])
	numBeats := binary.LittleEndian.Uint32(data[12:16])
	meterNum := binary.LittleEndian.Uint16(data[16:18])
	meterDen := binary.LittleEndian.Uint16(data[18:20])
	tempo := math.Float32frombits(binary.LittleEndian.Uint32(data[20:24]))

	if flags&0x04 != 0 {
		values["is_loop"] = "true"
		values["loop_class"] = "loop"
	} else if flags&0x01 != 0 {
		values["is_loop"] = "false"
		values["loop_class"] = "oneshot"
	}

	if tempo > 10 && tempo < 500 && !math.IsNaN(float64(tempo)) {
		values["bpm"] = fmt.Sprintf("%.2f", tempo)
	}
	if numBeats > 0 && numBeats <= 256 {
		values["num_beats"] = fmt.Sprintf("%d", numBeats)
	}
	if meterNum > 0 && meterDen > 0 {
		values["meter"] = fmt.Sprintf("%d/%d", meterNum, meterDen)
	}
	if (flags&0x02 != 0 || rootNote > 0) && rootNote <= 127 {
		values["midi_note"] = fmt.Sprintf("%d", rootNote)
		if name := MidiNoteToName(int(rootNote)); name != "" {
			values["note_name"] = name
		}
	}
}

func parseBextChunk(data []byte, values map[string]string) {
	if len(data) < 338 {
		return
	}
	desc := cleanNullTerminated(data[0:256])
	originator := cleanNullTerminated(data[256:288])
	date := cleanNullTerminated(data[320:330])
	origTime := cleanNullTerminated(data[330:338])

	addMetadataValue(values, "description", desc)
	addMetadataValue(values, "originator", originator)
	addMetadataValue(values, "origination_date", date)
	addMetadataValue(values, "origination_time", origTime)

	if len(data) >= 420 {
		version := binary.LittleEndian.Uint16(data[346:348])
		if version >= 1 {
			loudnessVal := int16(binary.LittleEndian.Uint16(data[412:414]))
			if loudnessVal != 0 {
				values["bext_loudness_lufs"] = fmt.Sprintf("%.2f", float64(loudnessVal)/100.0)
			}
			peakVal := int16(binary.LittleEndian.Uint16(data[416:418]))
			if peakVal != 0 {
				values["bext_true_peak_dbtp"] = fmt.Sprintf("%.2f", float64(peakVal)/100.0)
			}
		}
	}
}

func parseListChunk(data []byte, values map[string]string) {
	if len(data) < 4 {
		return
	}
	listType := string(data[:4])
	if listType != "INFO" {
		return
	}

	reader := bytes.NewReader(data[4:])
	for {
		var subID [4]byte
		var subSize uint32
		if _, err := io.ReadFull(reader, subID[:]); err != nil {
			break
		}
		if err := binary.Read(reader, binary.LittleEndian, &subSize); err != nil {
			break
		}
		subData := make([]byte, subSize)
		if _, err := io.ReadFull(reader, subData); err != nil {
			break
		}

		str := cleanNullTerminated(subData)
		switch string(subID[:]) {
		case "INAM":
			addMetadataValue(values, "title", str)
		case "IART":
			addMetadataValue(values, "artist", str)
		case "IPRD":
			addMetadataValue(values, "album", str)
		case "IGNR":
			addMetadataValue(values, "genre", str)
		case "ICMT":
			addMetadataValue(values, "comment", str)
		case "ICRD":
			addMetadataValue(values, "year", str)
		case "ICOP":
			addMetadataValue(values, "copyright", str)
		case "ISFT":
			addMetadataValue(values, "software", str)
		case "IENG":
			addMetadataValue(values, "engineer", str)
		case "IKEY":
			addMetadataValue(values, "key", str)
		case "TBPM":
			addMetadataValue(values, "bpm", str)
		}

		if subSize%2 != 0 {
			_, _ = reader.Seek(1, io.SeekCurrent)
		}
	}
}

func parseBascChunk(data []byte, values map[string]string) {
	if len(data) < 22 {
		return
	}
	loopType := binary.BigEndian.Uint32(data[4:8])
	numBeats := binary.BigEndian.Uint32(data[8:12])
	rootNote := binary.BigEndian.Uint16(data[12:14])
	scaleType := binary.BigEndian.Uint16(data[14:16])
	meterNum := binary.BigEndian.Uint16(data[18:20])
	meterDen := binary.BigEndian.Uint16(data[20:22])

	switch loopType {
	case 1:
		values["is_loop"] = "true"
		values["loop_class"] = "loop"
	case 0:
		values["is_loop"] = "false"
		values["loop_class"] = "oneshot"
	}

	if numBeats > 0 && numBeats <= 256 {
		values["num_beats"] = fmt.Sprintf("%d", numBeats)
	}
	if meterNum > 0 && meterDen > 0 {
		values["meter"] = fmt.Sprintf("%d/%d", meterNum, meterDen)
	}
	if rootNote <= 127 {
		values["midi_note"] = fmt.Sprintf("%d", rootNote)
		if name := MidiNoteToName(int(rootNote)); name != "" {
			values["note_name"] = name
			scaleName := ""
			switch scaleType {
			case 1:
				scaleName = "major"
			case 2:
				scaleName = "minor"
			}
			if scaleName != "" {
				values["scale_type"] = scaleName
				values["key"] = fmt.Sprintf("%s %s", name, scaleName)
			}
		}
	}
}

func parseID3v2Data(data []byte, values map[string]string) {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return
	}
	version := data[3]
	flags := data[5]
	tagSize := syncSafeInt(data[6:10])

	offset := 10
	if flags&0x40 != 0 && len(data) >= 14 { // Extended header
		extSize := syncSafeInt(data[10:14])
		offset += int(extSize)
	}

	limit := offset + int(tagSize)
	if limit > len(data) {
		limit = len(data)
	}

	for offset+10 <= limit {
		frameID := string(data[offset : offset+4])
		if frameID[0] == 0 {
			break
		}

		var frameSize int
		if version == 4 {
			frameSize = int(syncSafeInt(data[offset+4 : offset+8]))
		} else {
			frameSize = int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		}
		offset += 10
		if frameSize <= 0 || offset+frameSize > limit {
			break
		}

		frameData := data[offset : offset+frameSize]
		offset += frameSize

		// Skip encoding byte if present
		text := ""
		if len(frameData) > 1 {
			text = cleanNullTerminated(frameData[1:])
		} else if len(frameData) == 1 {
			text = cleanNullTerminated(frameData)
		}

		switch frameID {
		case "TBPM":
			addMetadataValue(values, "bpm", text)
		case "TKEY":
			addMetadataValue(values, "key", text)
		case "TIT2":
			addMetadataValue(values, "title", text)
		case "TPE1":
			addMetadataValue(values, "artist", text)
		case "TALB":
			addMetadataValue(values, "album", text)
		case "TCON":
			addMetadataValue(values, "genre", text)
		}
	}
}

func extractRawTags(raw map[string]interface{}, values map[string]string) {
	if raw == nil {
		return
	}
	for k, v := range raw {
		keyUpper := strings.ToUpper(strings.TrimSpace(k))
		strVal := fmt.Sprintf("%v", v)
		strVal = strings.TrimSpace(strVal)
		if strVal == "" || strVal == "<nil>" {
			continue
		}

		switch keyUpper {
		case "TBPM", "BPM", "TEMPO", "FBPM":
			if _, ok := values["bpm"]; !ok {
				values["bpm"] = strVal
			}
		case "TKEY", "KEY", "INITIALKEY", "INITIAL_KEY", "TONALITY":
			if _, ok := values["key"]; !ok {
				values["key"] = strVal
			}
		}
	}
}

var camelotToKey = map[string]struct{ tonic, mode string }{
	"1A":  {"Ab", "minor"},
	"1B":  {"B", "major"},
	"2A":  {"Eb", "minor"},
	"2B":  {"F#", "major"},
	"3A":  {"Bb", "minor"},
	"3B":  {"Db", "major"},
	"4A":  {"F", "minor"},
	"4B":  {"Ab", "major"},
	"5A":  {"C", "minor"},
	"5B":  {"Eb", "major"},
	"6A":  {"G", "minor"},
	"6B":  {"Bb", "major"},
	"7A":  {"D", "minor"},
	"7B":  {"F", "major"},
	"8A":  {"A", "minor"},
	"8B":  {"C", "major"},
	"9A":  {"E", "minor"},
	"9B":  {"G", "major"},
	"10A": {"B", "minor"},
	"10B": {"D", "major"},
	"11A": {"F#", "minor"},
	"11B": {"A", "major"},
	"12A": {"C#", "minor"},
	"12B": {"E", "major"},
}

// NormalizeMusicalKey normalizes common musical key representations
// (including Camelot wheel, e.g. "8B" -> ("C major", "C", "major")).
func NormalizeMusicalKey(input string) (key string, tonic string, mode string, ok bool) {
	clean := strings.TrimSpace(input)
	if clean == "" {
		return "", "", "", false
	}

	// 1. Check Camelot code (e.g. "8B", "11A")
	upper := strings.ToUpper(clean)
	if cam, exists := camelotToKey[upper]; exists {
		return fmt.Sprintf("%s %s", cam.tonic, cam.mode), cam.tonic, cam.mode, true
	}

	// 2. Parse traditional pitch name: [A-G][#|b]? [major|minor|maj|min|m]?
	raw := clean
	lower := strings.ToLower(raw)

	// Mode determination
	mode = "major"
	if strings.HasSuffix(lower, "minor") || strings.HasSuffix(lower, "min") || strings.HasSuffix(raw, "m") {
		mode = "minor"
	}

	// Extract root note
	letter := rune(raw[0])
	if letter < 'A' || letter > 'G' {
		// Try lowercase
		if letter >= 'a' && letter <= 'g' {
			letter = letter - 'a' + 'A'
		} else {
			return "", "", "", false
		}
	}

	tonic = string(letter)
	rest := raw[1:]
	if len(rest) > 0 && (rest[0] == '#' || rest[0] == 'b') {
		tonic += string(rest[0])
		rest = rest[1:]
	}

	// Check if mode was specified right after note without space, e.g. "F#m"
	restLower := strings.TrimSpace(strings.ToLower(rest))
	switch restLower {
	case "m", "min", "minor":
		mode = "minor"
	case "maj", "major":
		mode = "major"
	}

	key = fmt.Sprintf("%s %s", tonic, mode)
	return key, tonic, mode, true
}

var noteNames = []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// MidiNoteToName converts a MIDI note number (0-127) to standard note name (e.g. 60 -> "C4").
func MidiNoteToName(note int) string {
	if note < 0 || note > 127 {
		return ""
	}
	octave := (note / 12) - 1
	name := noteNames[note%12]
	return fmt.Sprintf("%s%d", name, octave)
}

// MidiToHz converts a MIDI note number to its fundamental frequency in Hz.
func MidiToHz(note int) float64 {
	if note < 0 || note > 127 {
		return 0
	}
	return 440.0 * math.Pow(2.0, float64(note-69)/12.0)
}

func syncSafeInt(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return (uint32(b[0]&0x7F) << 21) |
		(uint32(b[1]&0x7F) << 14) |
		(uint32(b[2]&0x7F) << 7) |
		uint32(b[3]&0x7F)
}

func cleanNullTerminated(b []byte) string {
	if idx := bytes.IndexByte(b, 0); idx >= 0 {
		b = b[:idx]
	}
	return cleanString(string(b))
}

func cleanString(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == 0 || r == '\r' {
			return -1
		}
		return r
	}, s))
}

func addMetadataValue(values map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	values[key] = value
}
