package clap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// minimalTokenizerJSON builds a minimal tokenizer.json suitable for testing.
func minimalTokenizerJSON(t *testing.T) string {
	t.Helper()
	vocab := map[string]int32{
		// Individual byte tokens for ASCII letters used in test words
		"a": 0, "m": 1, "b": 2, "i": 3, "e": 4, "n": 5, "t": 6,
		"Ġ": 7, // space (byte 32 → chr(288) = Ġ)
		// Merged tokens
		"am":      8,
		"bi":      9,
		"en":      10,
		"ambient": 11,
		"Ġbass":   12,
		// Special tokens
		"<|startoftext|>": 49406,
		"<|endoftext|>":   49407,
	}
	merges := []string{
		"a m",  // rank 0
		"b i",  // rank 1
		"e n",  // rank 2
	}

	data := map[string]any{
		"model": map[string]any{
			"type":   "BPE",
			"vocab":  vocab,
			"merges": merges,
		},
	}
	raw, _ := json.Marshal(data)

	dir := t.TempDir()
	path := filepath.Join(dir, "tokenizer.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write test tokenizer.json: %v", err)
	}
	return path
}

func TestNewBPETokenizer_LoadsVocab(t *testing.T) {
	path := minimalTokenizerJSON(t)
	tok, err := NewBPETokenizer(path, 0)
	if err != nil {
		t.Fatalf("NewBPETokenizer: %v", err)
	}
	if tok.VocabSize() == 0 {
		t.Error("vocabulary should be non-empty")
	}
	if tok.maxLen != DefaultMaxTokens {
		t.Errorf("maxLen = %d, want %d", tok.maxLen, DefaultMaxTokens)
	}
}

func TestNewBPETokenizer_DetectsSpecialTokens(t *testing.T) {
	path := minimalTokenizerJSON(t)
	tok, err := NewBPETokenizer(path, 0)
	if err != nil {
		t.Fatalf("NewBPETokenizer: %v", err)
	}
	if tok.bosID != 49406 {
		t.Errorf("bosID = %d, want 49406", tok.bosID)
	}
	if tok.eosID != 49407 {
		t.Errorf("eosID = %d, want 49407", tok.eosID)
	}
}

func TestNewBPETokenizer_InvalidFile(t *testing.T) {
	if _, err := NewBPETokenizer("/nonexistent/tokenizer.json", 0); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestNewBPETokenizer_NonBPEType(t *testing.T) {
	data := map[string]any{
		"model": map[string]any{
			"type":  "WordPiece",
			"vocab": map[string]int32{"a": 0},
		},
	}
	raw, _ := json.Marshal(data)
	dir := t.TempDir()
	path := filepath.Join(dir, "tokenizer.json")
	_ = os.WriteFile(path, raw, 0o600)
	if _, err := NewBPETokenizer(path, 0); err == nil {
		t.Error("expected error for non-BPE tokenizer type")
	}
}

func TestEncode_SequenceLength(t *testing.T) {
	path := minimalTokenizerJSON(t)
	tok, err := NewBPETokenizer(path, 10)
	if err != nil {
		t.Fatalf("NewBPETokenizer: %v", err)
	}

	ids, mask := tok.Encode("ambient")
	if len(ids) != 10 {
		t.Errorf("len(ids) = %d, want 10", len(ids))
	}
	if len(mask) != 10 {
		t.Errorf("len(mask) = %d, want 10", len(mask))
	}
	// First token is BOS (49406), last real token is EOS (49407).
	if ids[0] != 49406 {
		t.Errorf("ids[0] = %d, want BOS 49406", ids[0])
	}
}

func TestEncode_PaddingIsZero(t *testing.T) {
	path := minimalTokenizerJSON(t)
	tok, err := NewBPETokenizer(path, 20)
	if err != nil {
		t.Fatalf("NewBPETokenizer: %v", err)
	}
	ids, mask := tok.Encode("a")
	// Find the first zero in mask — that position and onwards must be zero.
	firstPad := -1
	for i, m := range mask {
		if m == 0 {
			firstPad = i
			break
		}
	}
	if firstPad >= 0 {
		for i := firstPad; i < len(mask); i++ {
			if mask[i] != 0 {
				t.Errorf("mask[%d] = %d after pad start, want 0", i, mask[i])
			}
			if ids[i] != 0 {
				t.Errorf("ids[%d] = %d after pad start, want 0", i, ids[i])
			}
		}
	}
}

func TestBytesToUnicode_SpaceMapsToGlyph(t *testing.T) {
	table := buildBytesToUnicode()
	// byte 32 (space) should map to chr(288) = Ġ
	if table[32] != 'Ġ' {
		t.Errorf("byte 32 → %c (U+%04X), want Ġ (U+0120)", table[32], table[32])
	}
	// Printable ASCII letters map to themselves.
	if table['a'] != 'a' {
		t.Errorf("byte 'a' should map to 'a'")
	}
}
