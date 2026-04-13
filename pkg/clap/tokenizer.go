package clap

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
)

// DefaultMaxTokens is the maximum token sequence length used by CLIP/CLAP.
const DefaultMaxTokens = 77

// bytesToUnicode builds the GPT-2 bytes→unicode mapping used by byte-level BPE.
// Every byte (0-255) is mapped to a unique rune.  Printable ASCII and a run of
// latin-1 code-points map to themselves; the remaining 68 bytes map to
// supplementary code-points starting at U+0100 (Ā).
func buildBytesToUnicode() [256]rune {
	var table [256]rune

	// Ranges that map to themselves
	inSet := [256]bool{}
	for b := '!'; b <= '~'; b++ {
		inSet[b] = true
		table[b] = b
	}
	for b := rune(0xA1); b <= 0xAC; b++ { // ¡ … ¬
		inSet[b] = true
		table[b] = b
	}
	for b := rune(0xAE); b <= 0xFF; b++ { // ® … ÿ
		inSet[b] = true
		table[b] = b
	}

	// All remaining bytes get supplementary code-points
	n := 0
	for b := 0; b < 256; b++ {
		if !inSet[b] {
			table[b] = rune(0x100 + n)
			n++
		}
	}
	return table
}

// tokenizerJSON is the minimal subset of a HuggingFace tokenizer.json that
// this package needs to decode.
type tokenizerJSON struct {
	AddedTokens   []addedTokenJSON    `json:"added_tokens"`
	PreTokenizer  *preTokenizerJSON   `json:"pre_tokenizer"`
	PostProcessor *postProcessorJSON  `json:"post_processor"`
	Model         bpeModelJSON        `json:"model"`
}

type addedTokenJSON struct {
	ID      int32  `json:"id"`
	Content string `json:"content"`
	Special bool   `json:"special"`
}

type preTokenizerJSON struct {
	Type           string `json:"type"`
	AddPrefixSpace bool   `json:"add_prefix_space"`
}

type postProcessorJSON struct {
	Type string            `json:"type"`
	CLS  []json.RawMessage `json:"cls"`
	SEP  []json.RawMessage `json:"sep"`
}

type bpeModelJSON struct {
	Type   string           `json:"type"`
	Vocab  map[string]int32 `json:"vocab"`
	Merges []string         `json:"merges"`
}

// BPETokenizer is a pure-Go byte-level BPE tokenizer that is compatible with
// the tokenizer.json format produced by the HuggingFace tokenizers library.
// It works with both CLIP-style and RoBERTa-style tokenizers.
type BPETokenizer struct {
	vocab          map[string]int32
	bpeRanks       map[[2]string]int
	bytesToUni     [256]rune
	bosID          int32
	eosID          int32
	maxLen         int
	addPrefixSpace bool
	cache          sync.Map // string → []string
}

// NewBPETokenizer loads a tokenizer from the HuggingFace tokenizer.json file
// at path.  maxLen is the maximum token sequence length including special
// tokens (0 uses DefaultMaxTokens).
func NewBPETokenizer(path string, maxLen int) (*BPETokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer file %q: %w", path, err)
	}

	var raw tokenizerJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}

	if raw.Model.Type != "" && !strings.EqualFold(raw.Model.Type, "BPE") {
		return nil, fmt.Errorf("unsupported tokenizer model type %q (expected BPE)", raw.Model.Type)
	}
	if len(raw.Model.Vocab) == 0 {
		return nil, fmt.Errorf("tokenizer.json: empty vocabulary")
	}

	if maxLen <= 0 {
		maxLen = DefaultMaxTokens
	}

	bpeRanks := make(map[[2]string]int, len(raw.Model.Merges))
	for i, m := range raw.Model.Merges {
		idx := strings.Index(m, " ")
		if idx < 0 {
			continue
		}
		bpeRanks[[2]string{m[:idx], m[idx+1:]}] = i
	}

	t := &BPETokenizer{
		vocab:      raw.Model.Vocab,
		bpeRanks:   bpeRanks,
		bytesToUni: buildBytesToUnicode(),
		maxLen:     maxLen,
	}

	if raw.PreTokenizer != nil {
		t.addPrefixSpace = raw.PreTokenizer.AddPrefixSpace
	}

	// Detect BOS / EOS token IDs from post_processor (RoBERTa/Template style).
	if raw.PostProcessor != nil && len(raw.PostProcessor.CLS) >= 2 {
		_, id, err := parseSpecialPair(raw.PostProcessor.CLS)
		if err == nil {
			t.bosID = id
		}
	}
	if raw.PostProcessor != nil && len(raw.PostProcessor.SEP) >= 2 {
		_, id, err := parseSpecialPair(raw.PostProcessor.SEP)
		if err == nil {
			t.eosID = id
		}
	}

	// Fall back to common special token names in the vocabulary.
	if t.bosID == 0 {
		for _, candidate := range []string{"<|startoftext|>", "<s>", "[CLS]"} {
			if id, ok := raw.Model.Vocab[candidate]; ok {
				t.bosID = id
				break
			}
		}
	}
	if t.eosID == 0 {
		for _, candidate := range []string{"<|endoftext|>", "</s>", "[SEP]"} {
			if id, ok := raw.Model.Vocab[candidate]; ok {
				t.eosID = id
				break
			}
		}
	}

	// Also check added_tokens for special token IDs by well-known names.
	if t.bosID == 0 || t.eosID == 0 {
		for _, at := range raw.AddedTokens {
			if !at.Special {
				continue
			}
			switch at.Content {
			case "<s>", "<|startoftext|>", "[CLS]":
				if t.bosID == 0 {
					t.bosID = at.ID
				}
			case "</s>", "<|endoftext|>", "[SEP]":
				if t.eosID == 0 {
					t.eosID = at.ID
				}
			}
		}
	}

	return t, nil
}

// parseSpecialPair parses a [string, number] JSON pair used in post_processor
// CLS/SEP fields.
func parseSpecialPair(raw []json.RawMessage) (string, int32, error) {
	var content string
	if err := json.Unmarshal(raw[0], &content); err != nil {
		return "", 0, err
	}
	var id int32
	if err := json.Unmarshal(raw[1], &id); err != nil {
		return "", 0, err
	}
	return content, id, nil
}

// Encode tokenises text using byte-level BPE and returns (input_ids,
// attention_mask) slices of length t.maxLen.  Positions beyond the actual
// token sequence are zero (pad id and mask).
func (t *BPETokenizer) Encode(text string) (ids, mask []int64) {
	ids = make([]int64, t.maxLen)
	mask = make([]int64, t.maxLen)
	pos := 0

	putToken := func(id int32) bool {
		if pos >= t.maxLen {
			return false
		}
		ids[pos] = int64(id)
		mask[pos] = 1
		pos++
		return true
	}

	// BOS
	putToken(t.bosID)

	// Split text into whitespace-delimited words.
	// Each word is byte-encoded and then BPE-merged.
	words := strings.Fields(strings.TrimSpace(text))
	for i, word := range words {
		if pos >= t.maxLen-1 { // leave room for EOS
			break
		}
		// Build the unicode-encoded word string.
		// Non-first words get a Ġ prefix (byte 0x20 → Ġ via bytes_to_unicode).
		var sb strings.Builder
		if i > 0 || t.addPrefixSpace {
			sb.WriteRune(t.bytesToUni[0x20]) // space → Ġ
		}
		for _, b := range []byte(word) {
			sb.WriteRune(t.bytesToUni[b])
		}
		encoded := sb.String()

		tokens := t.bpe(encoded)
		for _, tok := range tokens {
			if pos >= t.maxLen-1 {
				break
			}
			if id, ok := t.vocab[tok]; ok {
				if !putToken(id) {
					break
				}
			}
		}
	}

	// EOS
	putToken(t.eosID)

	return ids, mask
}

// bpe applies the byte-pair encoding algorithm to a single pre-tokenised word
// (already converted to unicode via bytes_to_unicode).
func (t *BPETokenizer) bpe(word string) []string {
	if result, ok := t.cache.Load(word); ok {
		return result.([]string)
	}

	// Initialise as individual runes.
	runes := []string{}
	for _, r := range word {
		runes = append(runes, string(r))
	}
	// Edge case: single character.
	if len(runes) <= 1 {
		t.cache.Store(word, runes)
		return runes
	}

	for {
		bestRank := math.MaxInt
		bestIdx := -1
		for i := 0; i < len(runes)-1; i++ {
			pair := [2]string{runes[i], runes[i+1]}
			if rank, ok := t.bpeRanks[pair]; ok && rank < bestRank {
				bestRank = rank
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break
		}
		merged := runes[bestIdx] + runes[bestIdx+1]
		next := make([]string, 0, len(runes)-1)
		for i := 0; i < len(runes); {
			if i == bestIdx {
				next = append(next, merged)
				i += 2
			} else {
				next = append(next, runes[i])
				i++
			}
		}
		runes = next
		if len(runes) == 1 {
			break
		}
	}

	t.cache.Store(word, runes)
	return runes
}

// VocabSize returns the size of the tokenizer vocabulary.
func (t *BPETokenizer) VocabSize() int {
	return len(t.vocab)
}
