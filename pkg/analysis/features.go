package analysis

import (
	"fmt"
	"sort"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

const featureVectorVersion = "0.7.0"

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
