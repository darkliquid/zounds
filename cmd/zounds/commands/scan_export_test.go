package commands_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

func TestScanCommandIndexesSamples(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	mustWriteFile(t, filepath.Join(root, "kicks", "kick.wav"))
	mustWriteFile(t, filepath.Join(root, "drums", "hat.mp3"))

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", dbPath, "scan", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	if !strings.Contains(out.String(), "indexed 2 audio files") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestScanCommandVerboseShowsScanAndDatabaseProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	mustWriteFile(t, filepath.Join(root, "kicks", "kick.wav"))

	cmd := commands.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--verbose", "--db", dbPath, "scan", root})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"verbose: scanning file " + filepath.Join(root, "kicks", "kick.wav"),
		"verbose: setting up database at " + dbPath,
		"verbose: applying migration migrations/0001_initial.sql",
		"indexed 1 audio files into " + dbPath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in output %q", want, output)
		}
	}
}

func TestExportCommandOutputsJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "export.db")
	mustWriteFile(t, filepath.Join(root, "fx", "zap.ogg"))

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}

	export := commands.NewRootCommand()
	var out bytes.Buffer
	export.SetOut(&out)
	export.SetErr(&out)
	export.SetArgs([]string{"--db", dbPath, "export", "--format", "json"})
	if err := export.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute export command: %v", err)
	}

	var payload struct {
		Samples []core.Sample `json:"samples"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode export JSON: %v", err)
	}
	if len(payload.Samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(payload.Samples))
	}
	if payload.Samples[0].Format != core.FormatOGG {
		t.Fatalf("unexpected sample format: %s", payload.Samples[0].Format)
	}
}

func TestExportCommandOutputsRichJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "export.db")
	mustWriteFile(t, filepath.Join(root, "fx", "zap.ogg"))

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}
	seedRichExport(t, dbPath)

	export := commands.NewRootCommand()
	var out bytes.Buffer
	export.SetOut(&out)
	export.SetErr(&out)
	export.SetArgs([]string{"--db", dbPath, "export", "--format", "json"})
	if err := export.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute export command: %v", err)
	}

	var payload struct {
		SchemaVersion int    `json:"schema_version"`
		ExportedAt    string `json:"exported_at"`
		Samples       []struct {
			ID          int64                   `json:"id"`
			FileName    string                  `json:"file_name"`
			Format      core.AudioFormat        `json:"format"`
			Tags        []core.Tag              `json:"tags"`
			Analysis    *core.SampleAnalysis    `json:"analysis"`
			Projections []core.SampleProjection `json:"projections"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode export JSON: %v", err)
	}
	if payload.SchemaVersion != 1 || payload.ExportedAt == "" {
		t.Fatalf("unexpected export envelope: %+v", payload)
	}
	if len(payload.Samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(payload.Samples))
	}
	sample := payload.Samples[0]
	if sample.Format != core.FormatOGG {
		t.Fatalf("unexpected sample format: %s", sample.Format)
	}
	if len(sample.Tags) != 1 || sample.Tags[0].NormalizedName != "instrument:kick" || sample.Tags[0].Source != "rules" {
		t.Fatalf("unexpected exported tags: %+v", sample.Tags)
	}
	if sample.Analysis == nil || sample.Analysis.BPM == nil || *sample.Analysis.BPM != 128 {
		t.Fatalf("unexpected exported analysis: %+v", sample.Analysis)
	}
	if len(sample.Projections) != 1 || sample.Projections[0].Method != "tsne" {
		t.Fatalf("unexpected exported projections: %+v", sample.Projections)
	}
}

func TestExportCommandOutputsRichCSV(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "export.db")
	mustWriteFile(t, filepath.Join(root, "drums", "kick.wav"))

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan command: %v", err)
	}
	seedRichExport(t, dbPath)

	export := commands.NewRootCommand()
	var out bytes.Buffer
	export.SetOut(&out)
	export.SetErr(&out)
	export.SetArgs([]string{"--db", dbPath, "export", "--format", "csv"})
	if err := export.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute export command: %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatalf("parse export CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header and 1 data row, got %d records", len(records))
	}
	columns := make(map[string]int, len(records[0]))
	for i, name := range records[0] {
		columns[name] = i
	}
	row := records[1]
	for _, name := range []string{
		"tags_json", "bpm", "chord", "metrics_json", "attributes_json", "projections_json",
	} {
		if _, ok := columns[name]; !ok {
			t.Fatalf("expected CSV column %q, got header %v", name, records[0])
		}
	}

	var tags []core.Tag
	if err := json.Unmarshal([]byte(row[columns["tags_json"]]), &tags); err != nil {
		t.Fatalf("decode tags_json: %v", err)
	}
	if len(tags) != 1 || tags[0].NormalizedName != "instrument:kick" || tags[0].Source != "rules" || tags[0].Confidence != 0.8 {
		t.Fatalf("unexpected tags_json: %s", row[columns["tags_json"]])
	}
	if row[columns["bpm"]] != "128" || row[columns["chord"]] != "Cmaj7" {
		t.Fatalf("unexpected analysis columns: bpm=%q chord=%q", row[columns["bpm"]], row[columns["chord"]])
	}
	if row[columns["metrics_json"]] != `{"spectral_flux":0.42}` {
		t.Fatalf("unexpected metrics column: %q", row[columns["metrics_json"]])
	}
	if row[columns["attributes_json"]] != `{"note_name":"C4"}` {
		t.Fatalf("unexpected attributes column: %q", row[columns["attributes_json"]])
	}

	var projections []core.SampleProjection
	if err := json.Unmarshal([]byte(row[columns["projections_json"]]), &projections); err != nil {
		t.Fatalf("decode projections_json: %v", err)
	}
	if len(projections) != 1 || projections[0].Method != "tsne" || projections[0].Namespace != "acoustic" || projections[0].X != 12.5 || projections[0].Y != -4.25 {
		t.Fatalf("unexpected projections_json: %s", row[columns["projections_json"]])
	}
}

func seedRichExport(t *testing.T, dbPath string) {
	t.Helper()

	ctx := context.Background()
	database, err := db.Open(ctx, db.Options{Path: dbPath})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = database.Close() }()
	repo := db.NewRepository(database)

	samples, err := repo.ListSamples(ctx)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 indexed sample, got %d", len(samples))
	}
	sample := samples[0]

	tagID, err := repo.EnsureTag(ctx, core.Tag{
		Name:           "instrument:kick",
		NormalizedName: "instrument:kick",
	})
	if err != nil {
		t.Fatalf("ensure tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, sample.ID, tagID, "rules", 0.8); err != nil {
		t.Fatalf("attach tag: %v", err)
	}

	bpm := 128.0
	chord := "Cmaj7"
	loopClass := "oneshot"
	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{
		SampleID:   sample.ID,
		BPM:        &bpm,
		Chord:      &chord,
		LoopClass:  &loopClass,
		Metrics:    map[string]float64{"spectral_flux": 0.42},
		Attributes: map[string]string{"note_name": "C4"},
	}); err != nil {
		t.Fatalf("upsert analysis: %v", err)
	}

	if err := repo.UpsertSampleProjection(ctx, core.SampleProjection{
		SampleID:  sample.ID,
		Method:    "tsne",
		Namespace: "acoustic",
		X:         12.5,
		Y:         -4.25,
	}); err != nil {
		t.Fatalf("upsert projection: %v", err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
