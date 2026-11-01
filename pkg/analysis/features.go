package analysis

import (
	"fmt"
	"sort"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

const featureVectorVersion = "0.8.0"

var defaultFeatureOrder = []string{
	"sample_rate",
	"channels",
	"bit_depth",
	"bitrate",
	"duration_seconds",
	"spectral_centroid_hz",
	"spectral_rolloff_hz",
	"spectral_flux",
	"zero_crossing_rate",
	"dominant_frequency_hz",
	"spectral_flatness",
	"spectral_bandwidth_hz",
	"spectral_contrast_0",
	"spectral_contrast_1",
	"spectral_contrast_2",
	"spectral_contrast_3",
	"spectral_contrast_4",
	"spectral_contrast_5",
	"spectral_contrast_6",
	"peak",
	"peak_dbfs",
	"rms",
	"rms_dbfs",
	"crest_factor",
	"integrated_lufs",
	"mean_abs",
	"dynamic_range_db",
	"attack_sharpness",
	"sustain_ratio",
	"transient_rate",
	"temporal_centroid",
	"harmonic_energy_ratio",
	"percussive_energy_ratio",
	"harmonic_percussive_ratio",
	"attack_time_ms",
	"decay_time_ms",
	"release_time_ms",
	"sustain_level_dbfs",
	"clipping_ratio",
	"dc_offset",
	"silence_ratio",
	"estimated_snr_db",
	"harmonic_ratio",
	"inharmonicity",
	"formant_1_hz",
	"formant_2_hz",
	"formant_3_hz",
	"formant_4_hz",
	"splice_count",
	"splice_max_strength",
	"splice_mean_strength",
	"tempo_bpm",
	"beat_period_seconds",
	"onset_strength",
	"beat_count_estimate",
	"frequency_hz",
	"midi_note",
	"confidence",
	"cents_from_note",
	"chroma_0",
	"chroma_1",
	"chroma_2",
	"chroma_3",
	"chroma_4",
	"chroma_5",
	"chroma_6",
	"chroma_7",
	"chroma_8",
	"chroma_9",
	"chroma_10",
	"chroma_11",
	"tonnetz_0",
	"tonnetz_1",
	"tonnetz_2",
	"tonnetz_3",
	"tonnetz_4",
	"tonnetz_5",
	"mfcc_0",
	"mfcc_1",
	"mfcc_2",
	"mfcc_3",
	"mfcc_4",
	"mfcc_5",
	"mfcc_6",
	"mfcc_7",
	"mfcc_8",
	"mfcc_9",
	"mfcc_10",
	"mfcc_11",
	"mfcc_12",
	"loop_boundary_similarity",
	"loop_tail_rms_ratio",
	"loop_periodicity_score",
	"loop_phase_alignment",
	"loop_confidence",
}

type FeatureVectorBuilder struct {
	order []string
}

func NewFeatureVectorBuilder(order []string) *FeatureVectorBuilder {
	if len(order) == 0 {
		order = append([]string(nil), defaultFeatureOrder...)
	} else {
		order = append([]string(nil), order...)
	}

	return &FeatureVectorBuilder{order: order}
}

func (b *FeatureVectorBuilder) Build(sampleID int64, results ...core.AnalysisResult) (core.FeatureVector, error) {
	if b == nil || len(b.order) == 0 {
		return core.FeatureVector{}, fmt.Errorf("feature vector builder is not initialized")
	}

	metrics := FlattenMetrics(results...)
	values := make([]float64, len(b.order))
	for i, key := range b.order {
		values[i] = metrics[key]
	}

	return core.FeatureVector{
		SampleID:   sampleID,
		Namespace:  "analysis",
		Version:    featureVectorVersion,
		Values:     values,
		Dimensions: len(values),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

func FlattenMetrics(results ...core.AnalysisResult) map[string]float64 {
	metrics := make(map[string]float64)
	for _, result := range results {
		for key, value := range result.Metrics {
			metrics[key] = value
		}
	}
	return metrics
}

func FeatureNames() []string {
	return append([]string(nil), defaultFeatureOrder...)
}

func SortedMetricNames(results ...core.AnalysisResult) []string {
	seen := map[string]struct{}{}
	for _, result := range results {
		for key := range result.Metrics {
			seen[key] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for key := range seen {
		names = append(names, key)
	}
	sort.Strings(names)
	return names
}

// AcousticFeatureNames returns the list of all acoustic feature names,
// strictly excluding container metadata (sample_rate, channels, bit_depth, bitrate).
func AcousticFeatureNames() []string {
	return append([]string(nil), defaultFeatureOrder[4:]...)
}

var timbreFeatureOrder = []string{
	"spectral_centroid_hz",
	"spectral_rolloff_hz",
	"spectral_flux",
	"zero_crossing_rate",
	"dominant_frequency_hz",
	"spectral_flatness",
	"spectral_bandwidth_hz",
	"spectral_contrast_0",
	"spectral_contrast_1",
	"spectral_contrast_2",
	"spectral_contrast_3",
	"spectral_contrast_4",
	"spectral_contrast_5",
	"spectral_contrast_6",
	"peak",
	"peak_dbfs",
	"rms",
	"rms_dbfs",
	"crest_factor",
	"integrated_lufs",
	"mean_abs",
	"dynamic_range_db",
	"clipping_ratio",
	"dc_offset",
	"silence_ratio",
	"estimated_snr_db",
	"harmonic_ratio",
	"inharmonicity",
	"formant_1_hz",
	"formant_2_hz",
	"formant_3_hz",
	"formant_4_hz",
	"splice_count",
	"splice_max_strength",
	"splice_mean_strength",
	"mfcc_0",
	"mfcc_1",
	"mfcc_2",
	"mfcc_3",
	"mfcc_4",
	"mfcc_5",
	"mfcc_6",
	"mfcc_7",
	"mfcc_8",
	"mfcc_9",
	"mfcc_10",
	"mfcc_11",
	"mfcc_12",
}

// TimbreFeatureNames returns the feature names comprising the timbral subspace.
func TimbreFeatureNames() []string {
	return append([]string(nil), timbreFeatureOrder...)
}

var harmonicFeatureOrder = []string{
	"frequency_hz",
	"midi_note",
	"confidence",
	"cents_from_note",
	"chroma_0",
	"chroma_1",
	"chroma_2",
	"chroma_3",
	"chroma_4",
	"chroma_5",
	"chroma_6",
	"chroma_7",
	"chroma_8",
	"chroma_9",
	"chroma_10",
	"chroma_11",
	"tonnetz_0",
	"tonnetz_1",
	"tonnetz_2",
	"tonnetz_3",
	"tonnetz_4",
	"tonnetz_5",
	"harmonic_energy_ratio",
	"harmonic_ratio",
	"inharmonicity",
}

// HarmonicFeatureNames returns the feature names comprising the harmonic subspace.
func HarmonicFeatureNames() []string {
	return append([]string(nil), harmonicFeatureOrder...)
}

var rhythmFeatureOrder = []string{
	"duration_seconds",
	"tempo_bpm",
	"beat_period_seconds",
	"onset_strength",
	"beat_count_estimate",
	"transient_rate",
	"attack_sharpness",
	"sustain_ratio",
	"attack_time_ms",
	"decay_time_ms",
	"release_time_ms",
	"sustain_level_dbfs",
	"temporal_centroid",
	"percussive_energy_ratio",
	"harmonic_percussive_ratio",
	"loop_boundary_similarity",
	"loop_tail_rms_ratio",
	"loop_periodicity_score",
	"loop_phase_alignment",
	"loop_confidence",
}

// RhythmFeatureNames returns the feature names comprising the rhythmic subspace.
func RhythmFeatureNames() []string {
	return append([]string(nil), rhythmFeatureOrder...)
}

// BuildSubspaceVectors creates the separate feature vectors for "acoustic",
// "timbre", "harmonic", "rhythm", and "perceptual" subspaces.
func BuildSubspaceVectors(sampleID int64, results ...core.AnalysisResult) []core.FeatureVector {
	metrics := FlattenMetrics(results...)
	now := time.Now().UTC()

	build := func(namespace string, order []string) core.FeatureVector {
		vals := make([]float64, len(order))
		for i, key := range order {
			vals[i] = metrics[key]
		}
		return core.FeatureVector{
			SampleID:   sampleID,
			Namespace:  namespace,
			Version:    featureVectorVersion,
			Dimensions: len(vals),
			Values:     vals,
			CreatedAt:  now,
		}
	}

	subspaces := []core.FeatureVector{
		build("acoustic", AcousticFeatureNames()),
		build("timbre", TimbreFeatureNames()),
		build("harmonic", HarmonicFeatureNames()),
		build("rhythm", RhythmFeatureNames()),
	}

	for _, res := range results {
		if res.Analyzer == "perceptual_hash" {
			hashHex := res.Attributes["perceptual_hash"]
			if hashHex != "" {
				bits64 := make([]float64, 64)
				var val uint64
				if _, err := fmt.Sscanf(hashHex, "%016x", &val); err == nil {
					for b := 0; b < 64; b++ {
						if (val & (uint64(1) << uint(b))) != 0 {
							bits64[b] = 1.0
						}
					}
					subspaces = append(subspaces, core.FeatureVector{
						SampleID:   sampleID,
						Namespace:  "perceptual",
						Version:    res.Version,
						Dimensions: 64,
						Values:     bits64,
						CreatedAt:  now,
					})
				}
			}
			break
		}
	}

	return subspaces
}
