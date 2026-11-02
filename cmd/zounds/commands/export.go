package commands

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func newExportCommand(cfg *Config) *cobra.Command {
	var (
		format string
		output string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export samples with analyses, tags, and projections",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			export, err := pipeline.ExportLibrary(ctx, repo)
			if err != nil {
				return err
			}

			writer, writerCloser, err := outputWriter(cmd.OutOrStdout(), output)
			if err != nil {
				return err
			}

			var writeErr error
			switch format {
			case "json":
				encoder := json.NewEncoder(writer)
				encoder.SetIndent("", "  ")
				writeErr = encoder.Encode(export)
			case "csv":
				writeErr = exportCSV(writer, export)
			default:
				writeErr = fmt.Errorf("unsupported export format %q", format)
			}

			closeErr := writerCloser.Close()
			if writeErr != nil {
				if closeErr != nil {
					return fmt.Errorf("%w; close export output: %v", writeErr, closeErr)
				}
				return writeErr
			}
			if closeErr != nil {
				return fmt.Errorf("close export output: %w", closeErr)
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&format, "format", "json", "export format: json or csv")
	flags.StringVarP(&output, "output", "o", "-", "output file path or - for stdout")

	return cmd
}

func exportCSV(writer io.Writer, export pipeline.LibraryExport) error {
	w := csv.NewWriter(writer)
	return writeSampleCSV(w, export.Samples)
}

func writeSampleCSV(w *csv.Writer, samples []pipeline.ExportSample) error {
	if err := w.Write([]string{
		"id",
		"source_root",
		"path",
		"relative_path",
		"file_name",
		"extension",
		"format",
		"size_bytes",
		"modified_at",
		"scanned_at",
		"tags_json",
		"duration_seconds",
		"sample_rate",
		"channels",
		"bit_depth",
		"bpm",
		"bpm_confidence",
		"key_tonic",
		"key_mode",
		"key_confidence",
		"chord",
		"chord_confidence",
		"pitch_hz",
		"midi_note",
		"cents_offset",
		"pitch_confidence",
		"loop_class",
		"loop_confidence",
		"lufs_integrated",
		"true_peak_dbtp",
		"metrics_json",
		"attributes_json",
		"projections_json",
	}); err != nil {
		return err
	}

	for _, sample := range samples {
		analysis := core.SampleAnalysis{}
		if sample.Analysis != nil {
			analysis = *sample.Analysis
		}

		metricsJSON, err := formatJSONValue(analysis.Metrics, "{}")
		if err != nil {
			return fmt.Errorf("format metrics for sample %d: %w", sample.ID, err)
		}
		attributesJSON, err := formatJSONValue(analysis.Attributes, "{}")
		if err != nil {
			return fmt.Errorf("format attributes for sample %d: %w", sample.ID, err)
		}
		tagsJSON, err := formatJSONValue(sample.Tags, "[]")
		if err != nil {
			return fmt.Errorf("format tags for sample %d: %w", sample.ID, err)
		}
		projectionsJSON, err := formatJSONValue(sample.Projections, "[]")
		if err != nil {
			return fmt.Errorf("format projections for sample %d: %w", sample.ID, err)
		}

		if err := w.Write([]string{
			strconv.FormatInt(sample.ID, 10),
			sample.SourceRoot,
			sample.Path,
			sample.RelativePath,
			sample.FileName,
			sample.Extension,
			string(sample.Format),
			strconv.FormatInt(sample.SizeBytes, 10),
			formatTime(sample.ModifiedAt),
			formatTime(sample.ScannedAt),
			tagsJSON,
			formatOptionalFloat(analysis.DurationSeconds),
			formatOptionalInt(analysis.SampleRate),
			formatOptionalInt(analysis.Channels),
			formatOptionalInt(analysis.BitDepth),
			formatOptionalFloat(analysis.BPM),
			formatOptionalFloat(analysis.BPMConfidence),
			formatOptionalString(analysis.KeyTonic),
			formatOptionalString(analysis.KeyMode),
			formatOptionalFloat(analysis.KeyConfidence),
			formatOptionalString(analysis.Chord),
			formatOptionalFloat(analysis.ChordConfidence),
			formatOptionalFloat(analysis.PitchHz),
			formatOptionalInt(analysis.MidiNote),
			formatOptionalFloat(analysis.CentsOffset),
			formatOptionalFloat(analysis.PitchConfidence),
			formatOptionalString(analysis.LoopClass),
			formatOptionalFloat(analysis.LoopConfidence),
			formatOptionalFloat(analysis.LUFSIntegrated),
			formatOptionalFloat(analysis.TruePeakDBTP),
			metricsJSON,
			attributesJSON,
			projectionsJSON,
		}); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func formatJSONValue(value any, emptyValue string) (string, error) {
	if value == nil {
		return emptyValue, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return emptyValue, nil
	}
	return string(data), nil
}

func formatOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func formatOptionalInt(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
