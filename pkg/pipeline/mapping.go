package pipeline

import (
	"fmt"
	"math"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

// SampleAnalysisFromResult maps a combined AnalysisResult (flattened metrics
// and merged attributes) onto the indexed sample_analyses schema fields.
// analyzedModifiedAt records the file modification time the analysis ran
// against so incremental re-runs can skip unchanged files.
// Unknown metrics and attributes are preserved verbatim in the JSON columns.
func SampleAnalysisFromResult(result core.AnalysisResult, analyzedModifiedAt time.Time) core.SampleAnalysis {
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
	analysis.AnalyzedModifiedAt = analyzedModifiedAt

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

// AnalysisResultFromSampleAnalysis reconstructs an AnalysisResult from a
// persisted SampleAnalysis so taggers and pipeline consumers can access
// metrics and attributes without re-decoding audio files from disk.
func AnalysisResultFromSampleAnalysis(analysis core.SampleAnalysis) core.AnalysisResult {
	metrics := make(map[string]float64, len(analysis.Metrics)+16)
	for k, v := range analysis.Metrics {
		metrics[k] = v
	}

	if analysis.DurationSeconds != nil {
		setIfMissing(metrics, "duration_seconds", *analysis.DurationSeconds)
	}
	if analysis.SampleRate != nil {
		setIfMissing(metrics, "sample_rate", float64(*analysis.SampleRate))
	}
	if analysis.Channels != nil {
		setIfMissing(metrics, "channels", float64(*analysis.Channels))
	}
	if analysis.BitDepth != nil {
		setIfMissing(metrics, "bit_depth", float64(*analysis.BitDepth))
	}
	if analysis.BPM != nil {
		setIfMissing(metrics, "tempo_bpm", *analysis.BPM)
	}
	if analysis.BPMConfidence != nil {
		setIfMissing(metrics, "tempo_confidence", *analysis.BPMConfidence)
	}
	if analysis.KeyConfidence != nil {
		setIfMissing(metrics, "key_confidence", *analysis.KeyConfidence)
	}
	if analysis.ChordConfidence != nil {
		setIfMissing(metrics, "chord_confidence", *analysis.ChordConfidence)
	}
	if analysis.PitchHz != nil {
		setIfMissing(metrics, "frequency_hz", *analysis.PitchHz)
	}
	if analysis.MidiNote != nil {
		setIfMissing(metrics, "midi_note", float64(*analysis.MidiNote))
	}
	if analysis.CentsOffset != nil {
		setIfMissing(metrics, "cents_from_note", *analysis.CentsOffset)
	}
	if analysis.PitchConfidence != nil {
		setIfMissing(metrics, "confidence", *analysis.PitchConfidence)
	}
	if analysis.LoopConfidence != nil {
		setIfMissing(metrics, "loop_confidence", *analysis.LoopConfidence)
	}
	if analysis.LUFSIntegrated != nil {
		setIfMissing(metrics, "integrated_lufs", *analysis.LUFSIntegrated)
	}
	if analysis.TruePeakDBTP != nil {
		setIfMissing(metrics, "true_peak_dbtp", *analysis.TruePeakDBTP)
	}

	attributes := make(map[string]string, len(analysis.Attributes)+8)
	for k, v := range analysis.Attributes {
		attributes[k] = v
	}

	if analysis.KeyTonic != nil {
		setIfMissingAttr(attributes, "tonic", *analysis.KeyTonic)
	}
	if analysis.KeyMode != nil {
		setIfMissingAttr(attributes, "mode", *analysis.KeyMode)
	}
	if analysis.KeyTonic != nil && analysis.KeyMode != nil {
		setIfMissingAttr(attributes, "key", *analysis.KeyTonic+" "+*analysis.KeyMode)
	}
	if analysis.Chord != nil {
		setIfMissingAttr(attributes, "chord", *analysis.Chord)
	}
	if analysis.LoopClass != nil {
		setIfMissingAttr(attributes, "loop_class", *analysis.LoopClass)
	}

	return core.AnalysisResult{
		SampleID:   analysis.SampleID,
		Analyzer:   "persisted",
		Metrics:    metrics,
		Attributes: attributes,
	}
}

// AttributesFromSampleAnalysis returns a string-string map of attributes
// suitable for template rendering, combining analysis.Attributes with
// key, chord, loop, note, bpm, and embedded metadata fields.
func AttributesFromSampleAnalysis(analysis core.SampleAnalysis) map[string]string {
	result := AnalysisResultFromSampleAnalysis(analysis)
	attrs := result.Attributes

	if analysis.BPM != nil {
		if _, ok := attrs["bpm"]; !ok {
			attrs["bpm"] = fmt.Sprintf("%.1f", *analysis.BPM)
		}
	}
	return attrs
}

func setIfMissing(m map[string]float64, key string, val float64) {
	if _, ok := m[key]; !ok {
		m[key] = val
	}
}

func setIfMissingAttr(m map[string]string, key string, val string) {
	if _, ok := m[key]; !ok && val != "" {
		m[key] = val
	}
}

