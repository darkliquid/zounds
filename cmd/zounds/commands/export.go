package commands

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
)

func newExportCommand(cfg *Config) *cobra.Command {
	var (
		format string
		output string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export sample metadata and analysis results",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := repo.ListSamples(ctx)
			if err != nil {
				return err
			}

			writer, writerCloser, err := outputWriter(cmd.OutOrStdout(), output)
			if err != nil {
				return err
			}
			defer func() { _ = writerCloser.Close() }()

			switch format {
			case "json":
				encoder := json.NewEncoder(writer)
				encoder.SetIndent("", "  ")
				return encoder.Encode(samples)
			case "csv":
				return exportCSV(writer, samples)
			default:
				return fmt.Errorf("unsupported export format %q", format)
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&format, "format", "json", "export format: json or csv")
	flags.StringVarP(&output, "output", "o", "-", "output file path or - for stdout")

	return cmd
}

func exportCSV(writer io.Writer, samples []core.Sample) error {
	w := csv.NewWriter(writer)
	return writeSampleCSV(w, samples)
}

func writeSampleCSV(w *csv.Writer, samples []core.Sample) error {
	defer w.Flush()

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
	}); err != nil {
		return err
	}

	for _, sample := range samples {
		if err := w.Write([]string{
			fmt.Sprintf("%d", sample.ID),
			sample.SourceRoot,
			sample.Path,
			sample.RelativePath,
			sample.FileName,
			sample.Extension,
			string(sample.Format),
			fmt.Sprintf("%d", sample.SizeBytes),
			formatTime(sample.ModifiedAt),
			formatTime(sample.ScannedAt),
		}); err != nil {
			return err
		}
	}

	return w.Error()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
