package commands

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

type failingExportWriter struct{}

func (failingExportWriter) Write([]byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestWriteSampleCSVReturnsFlushError(t *testing.T) {
	t.Parallel()

	writer := csv.NewWriter(failingExportWriter{})
	err := writeSampleCSV(writer, nil)
	if err == nil {
		t.Fatalf("expected CSV flush error")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected underlying writer error, got %v", err)
	}
}

func TestWriteSampleCSVEncodesEmptyRichData(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := writeSampleCSV(csv.NewWriter(&out), []pipeline.ExportSample{{
		Sample: core.Sample{ID: 1, Path: "/library/tone.wav", FileName: "tone.wav"},
	}})
	if err != nil {
		t.Fatalf("write CSV: %v", err)
	}

	records, err := csv.NewReader(&out).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header and one row, got %d records", len(records))
	}
	columns := make(map[string]int, len(records[0]))
	for i, name := range records[0] {
		columns[name] = i
	}
	for name, want := range map[string]string{
		"tags_json":        "[]",
		"metrics_json":     "{}",
		"attributes_json":  "{}",
		"projections_json": "[]",
	} {
		if got := records[1][columns[name]]; got != want {
			t.Fatalf("unexpected %s: got %q, want %q", name, got, want)
		}
	}
}
