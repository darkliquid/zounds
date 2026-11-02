package pipeline_test

import (
	"context"
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/pipeline"
)

func TestExportLibraryIncludesRichSampleData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := openTestRepo(t)

	sampleID, err := repo.UpsertSample(ctx, core.Sample{
		SourceRoot:   "/library",
		Path:         "/library/drums/kick.wav",
		RelativePath: "drums/kick.wav",
		FileName:     "kick.wav",
		Extension:    "wav",
		Format:       core.FormatWAV,
		SizeBytes:    2048,
	})
	if err != nil {
		t.Fatalf("upsert sample: %v", err)
	}

	tagID, err := repo.EnsureTag(ctx, core.Tag{
		Name:           "instrument:kick",
		NormalizedName: "instrument:kick",
	})
	if err != nil {
		t.Fatalf("ensure tag: %v", err)
	}
	if err := repo.AttachTagToSample(ctx, sampleID, tagID, "rules", 0.8); err != nil {
		t.Fatalf("attach tag: %v", err)
	}

	duration := 1.5
	bpm := 128.0
	chord := "Cmaj7"
	loopClass := "oneshot"
	if err := repo.UpsertSampleAnalysis(ctx, core.SampleAnalysis{
		SampleID:        sampleID,
		DurationSeconds: &duration,
		BPM:             &bpm,
		Chord:           &chord,
		LoopClass:       &loopClass,
		Metrics:         map[string]float64{"spectral_flux": 0.42},
		Attributes:      map[string]string{"note_name": "C4"},
	}); err != nil {
		t.Fatalf("upsert analysis: %v", err)
	}

	if err := repo.UpsertSampleProjection(ctx, core.SampleProjection{
		SampleID:  sampleID,
		Method:    "tsne",
		Namespace: "acoustic",
		X:         12.5,
		Y:         -4.25,
	}); err != nil {
		t.Fatalf("upsert projection: %v", err)
	}

	exported, err := pipeline.ExportLibrary(ctx, repo)
	if err != nil {
		t.Fatalf("ExportLibrary: %v", err)
	}
	if exported.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", exported.SchemaVersion)
	}
	if exported.ExportedAt.IsZero() {
		t.Fatalf("expected exported_at timestamp to be set")
	}
	if len(exported.Samples) != 1 {
		t.Fatalf("expected 1 exported sample, got %d", len(exported.Samples))
	}

	sample := exported.Samples[0]
	if sample.ID != sampleID || sample.FileName != "kick.wav" {
		t.Fatalf("unexpected exported sample: %+v", sample.Sample)
	}
	if len(sample.Tags) != 1 || sample.Tags[0].NormalizedName != "instrument:kick" || sample.Tags[0].Source != "rules" || sample.Tags[0].Confidence != 0.8 {
		t.Fatalf("expected provenance-rich tags, got %+v", sample.Tags)
	}
	if sample.Analysis == nil {
		t.Fatalf("expected analysis to be exported")
	}
	if sample.Analysis.DurationSeconds == nil || *sample.Analysis.DurationSeconds != duration {
		t.Fatalf("expected duration to be exported, got %+v", sample.Analysis)
	}
	if sample.Analysis.BPM == nil || *sample.Analysis.BPM != bpm {
		t.Fatalf("expected BPM to be exported, got %+v", sample.Analysis)
	}
	if sample.Analysis.Metrics["spectral_flux"] != 0.42 || sample.Analysis.Attributes["note_name"] != "C4" {
		t.Fatalf("expected analysis metrics and attributes to be exported, got %+v", sample.Analysis)
	}
	if len(sample.Projections) != 1 {
		t.Fatalf("expected 1 projection, got %+v", sample.Projections)
	}
	projection := sample.Projections[0]
	if projection.Method != "tsne" || projection.Namespace != "acoustic" || projection.X != 12.5 || projection.Y != -4.25 {
		t.Fatalf("unexpected projection: %+v", projection)
	}
}
