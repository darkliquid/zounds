package tags

import (
	"context"
	"math"
	"strings"

	"github.com/darkliquid/zounds/pkg/clap"
	"github.com/darkliquid/zounds/pkg/core"
)

// Standard taxonomy facet namespaces.
const (
	FacetInstrument   = "instrument"
	FacetRole         = "role"
	FacetCharacter    = "character"
	FacetArticulation = "articulation"
	FacetGenre        = "genre"
)

// StandardFacets lists all recognized standard facet names.
var StandardFacets = []string{
	FacetInstrument,
	FacetRole,
	FacetCharacter,
	FacetArticulation,
	FacetGenre,
}

// DefaultFacetedLabels is the canonical namespaced tag taxonomy used by
// default in CLAP auto-tagging.
var DefaultFacetedLabels = []string{
	// Instruments
	"instrument:kick",
	"instrument:snare",
	"instrument:hihat",
	"instrument:clap",
	"instrument:cymbal",
	"instrument:tom",
	"instrument:percussion",
	"instrument:bass",
	"instrument:sub_bass",
	"instrument:synth",
	"instrument:piano",
	"instrument:guitar",
	"instrument:pad",
	"instrument:lead",
	"instrument:vocal",
	"instrument:fx",
	// Roles
	"role:lead",
	"role:pad",
	"role:bassline",
	"role:chord",
	"role:melody",
	"role:rhythm",
	"role:fill",
	"role:fx",
	// Characters
	"character:dark",
	"character:bright",
	"character:warm",
	"character:metallic",
	"character:punchy",
	"character:acoustic",
	"character:electronic",
	"character:distorted",
	"character:clean",
	"character:ambient",
	"character:glitch",
	"character:lofi",
	// Articulations
	"articulation:oneshot",
	"articulation:loop",
	"articulation:stab",
	"articulation:pluck",
	"articulation:sweep",
	// Genres
	"genre:techno",
	"genre:house",
	"genre:dnb",
	"genre:hiphop",
	"genre:trap",
	"genre:ambient",
	"genre:synthwave",
	"genre:dubstep",
}

// ParseTag splits a tag into its facet and value components if formatted as "facet:value".
// If no colon is present, it returns ("", tag, false).
func ParseTag(tag string) (facet, value string, isFaceted bool) {
	tag = core.NormalizeTagName(tag)
	idx := strings.Index(tag, ":")
	if idx < 0 {
		return "", tag, false
	}
	return tag[:idx], tag[idx+1:], true
}

// FormatTag creates a namespaced tag in the form "facet:value".
func FormatTag(facet, value string) string {
	return core.NormalizeTagName(facet + ":" + value)
}

// IsStandardFacet reports whether the given facet name is one of the recognized standard facets.
func IsStandardFacet(facet string) bool {
	f := strings.ToLower(strings.TrimSpace(facet))
	for _, std := range StandardFacets {
		if f == std {
			return true
		}
	}
	return false
}

// PromptEnsembleForTag generates a list of natural-language prompts for zero-shot
// audio-language models (e.g. CLAP) to represent the given tag with high semantic precision.
func PromptEnsembleForTag(tag string) []string {
	normalized := core.NormalizeTagName(tag)
	if normalized == "" {
		return nil
	}

	facet, val, isFaceted := ParseTag(normalized)
	if !isFaceted {
		valReadable := strings.ReplaceAll(normalized, "_", " ")
		return []string{
			"a sound of " + valReadable,
			"a " + valReadable + " audio sample",
			"an audio recording of " + valReadable,
			valReadable,
		}
	}

	valReadable := strings.ReplaceAll(val, "_", " ")
	switch facet {
	case FacetInstrument:
		return []string{
			"a sound of a " + valReadable,
			"a " + valReadable + " sample",
			"a " + valReadable + " one-shot sound",
			"a recording of a " + valReadable,
			valReadable + " sound",
		}
	case FacetRole:
		return []string{
			"a musical " + valReadable + " part",
			"a " + valReadable + " audio track",
			"a " + valReadable + " sound",
			valReadable + " sample",
		}
	case FacetCharacter:
		return []string{
			"a " + valReadable + " sounding sample",
			"an audio clip with " + valReadable + " timbre",
			"a " + valReadable + " tone",
			valReadable + " audio",
		}
	case FacetArticulation:
		switch val {
		case "oneshot":
			return []string{
				"a one-shot sample",
				"a single hit audio sound",
				"a short one-shot clip",
				"a dry one-shot hit",
			}
		case "loop":
			return []string{
				"a seamless loop audio",
				"a repeating musical loop",
				"a rhythm loop sample",
				"a continuous audio loop",
			}
		default:
			return []string{
				"a " + valReadable + " articulation sound",
				"a " + valReadable + " sample",
				valReadable + " playing technique",
			}
		}
	case FacetGenre:
		return []string{
			"a " + valReadable + " style sample",
			"a " + valReadable + " music beat",
			"a " + valReadable + " track element",
			valReadable + " sound",
		}
	default:
		return []string{
			"a " + facet + " " + valReadable + " sound",
			"a " + valReadable + " sample",
			valReadable,
		}
	}
}

// EnsembleTextEmbedding computes an L2-normalized average text embedding across
// the prompt ensemble for the given tag using model.EmbedText.
func EnsembleTextEmbedding(ctx context.Context, model *clap.Model, tag string) ([]float32, error) {
	prompts := PromptEnsembleForTag(tag)
	if len(prompts) == 0 {
		return model.EmbedText(ctx, tag)
	}

	var ensemble []float32
	for _, prompt := range prompts {
		emb, err := model.EmbedText(ctx, prompt)
		if err != nil {
			return nil, err
		}
		if ensemble == nil {
			ensemble = make([]float32, len(emb))
		}
		for i, v := range emb {
			ensemble[i] += v
		}
	}

	// L2-normalize the averaged ensemble vector
	var sumSq float64
	for _, v := range ensemble {
		sumSq += float64(v * v)
	}
	norm := float32(math.Sqrt(sumSq))
	if norm > 1e-12 {
		invNorm := 1.0 / norm
		for i := range ensemble {
			ensemble[i] *= invNorm
		}
	}

	return ensemble, nil
}
