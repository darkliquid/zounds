package pipeline

import (
	"math"

	"github.com/darkliquid/zounds/pkg/core"
)

// SampleAnalysisFromResult maps a combined AnalysisResult (flattened metrics
// and merged attributes) onto the indexed sample_analyses schema fields.
// Unknown metrics and attributes are preserved verbatim in the JSON columns.
func SampleAnalysisFromResult(result core.AnalysisResult) core.SampleAnalysis {
	analysis := core.SampleAnalysis{
		SampleID:   result.SampleID,
		Metrics:    result.Metrics,
		Attributes: result.Attributes,
	}

	analysis.DurationSeconds = metricPtr(result.Metrics, "duration_seconds")
	analysis.SampleRate = intMetricPtr(result.Metrics, "sample_rate")
	analysis.Channels = intMetricPtr(result.Metrics, "channels")
	analysis.BitDepth = intMetricPtr(result.Metrics, "bit_depth")
	analysis.BPM = metricPtr(result.Metrics, "tempo_bpm")
	analysis.BPMConfidence = metricPtr(result.Metrics, "tempo_confidence")
	analysis.KeyTonic = attrPtr(result.Attributes, "tonic")
	analysis.KeyMode = attrPtr(result.Attributes, "mode")
	analysis.KeyConfidence = metricPtr(result.Metrics, "key_confidence")
	analysis.Chord = attrPtr(result.Attributes, "chord")
	analysis.ChordConfidence = metricPtr(result.Metrics, "chord_confidence")
	analysis.PitchHz = metricPtr(result.Metrics, "frequency_hz")
	analysis.MidiNote = intMetricPtr(result.Metrics, "midi_note")
	analysis.CentsOffset = metricPtr(result.Metrics, "cents_from_note")
	analysis.PitchConfidence = metricPtr(result.Metrics, "confidence")
	analysis.LoopClass = attrPtr(result.Attributes, "loop_class")
	analysis.LoopConfidence = metricPtr(result.Metrics, "loop_confidence")
	analysis.LUFSIntegrated = metricPtr(result.Metrics, "integrated_lufs")
	analysis.TruePeakDBTP = metricPtr(result.Metrics, "true_peak_dbtp")
	analysis.AnalyzedModifiedAt = result.CompletedAt

	return analysis
}

func metricPtr(metrics map[string]float64, key string) *float64 {
	if metrics == nil {
		return nil
	}
	value, ok := metrics[key]
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func intMetricPtr(metrics map[string]float64, key string) *int64 {
	value := metricPtr(metrics, key)
	if value == nil {
		return nil
	}
	asInt := int64(math.Round(*value))
	return &asInt
}

func attrPtr(attributes map[string]string, key string) *string {
	if attributes == nil {
		return nil
	}
	value, ok := attributes[key]
	if !ok || value == "" {
		return nil
	}
	return &value
}
