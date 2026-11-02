package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

// ExportSchemaVersion is the schema version emitted by rich library exports.
const ExportSchemaVersion = 1

// LibraryExport is a complete, denormalized snapshot of persisted sample data.
type LibraryExport struct {
	SchemaVersion int            `json:"schema_version"`
	ExportedAt    time.Time      `json:"exported_at"`
	Samples       []ExportSample `json:"samples"`
}

// ExportSample is one sample with its persisted tags, analysis, and projections.
type ExportSample struct {
	core.Sample
	Tags        []core.Tag              `json:"tags,omitempty"`
	Analysis    *core.SampleAnalysis    `json:"analysis,omitempty"`
	Projections []core.SampleProjection `json:"projections,omitempty"`
}

// ExportLibrary loads every indexed sample with its persisted analyses, tags,
// and 2D projections using batched repository queries.
func ExportLibrary(ctx context.Context, repo *db.Repository) (LibraryExport, error) {
	if repo == nil {
		return LibraryExport{}, fmt.Errorf("pipeline: nil repository")
	}

	samples, err := repo.ListSamples(ctx)
	if err != nil {
		return LibraryExport{}, err
	}

	sampleIDs := make([]int64, len(samples))
	for i, sample := range samples {
		sampleIDs[i] = sample.ID
	}

	tagsBySample, err := repo.ListTagsForSampleIDs(ctx, sampleIDs)
	if err != nil {
		return LibraryExport{}, err
	}
	analysesBySample, err := repo.ListSampleAnalysesBySampleIDs(ctx, sampleIDs)
	if err != nil {
		return LibraryExport{}, err
	}
	projections, err := repo.ListSampleProjections(ctx, "", "")
	if err != nil {
		return LibraryExport{}, err
	}
	projectionsBySample := make(map[int64][]core.SampleProjection, len(projections))
	for _, projection := range projections {
		projectionsBySample[projection.SampleID] = append(projectionsBySample[projection.SampleID], projection)
	}

	exportedSamples := make([]ExportSample, 0, len(samples))
	for _, sample := range samples {
		item := ExportSample{
			Sample:      sample,
			Tags:        tagsBySample[sample.ID],
			Projections: projectionsBySample[sample.ID],
		}
		if analysis, ok := analysesBySample[sample.ID]; ok {
			analysisCopy := analysis
			item.Analysis = &analysisCopy
		}
		exportedSamples = append(exportedSamples, item)
	}

	return LibraryExport{
		SchemaVersion: ExportSchemaVersion,
		ExportedAt:    time.Now().UTC(),
		Samples:       exportedSamples,
	}, nil
}
