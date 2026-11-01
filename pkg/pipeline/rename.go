package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
	zrename "github.com/darkliquid/zounds/pkg/rename"
)

// PlanRename constructs the list of rename operations for the given samples
// using the provided Go template. For each sample, it retrieves existing tags
// and any stored SampleAnalysis so .Tags, .Attributes, and .Metadata are available
// to the template. It validates the plan against destination collisions.
func PlanRename(ctx context.Context, repo *db.Repository, samples []core.Sample, templateText string) ([]zrename.RenameOp, error) {
	if repo == nil {
		return nil, errors.New("pipeline: nil repository")
	}

	ops := make([]zrename.RenameOp, 0, len(samples))
	for _, sample := range samples {
		tags, err := repo.ListTagsForSample(ctx, sample.ID)
		if err != nil {
			return nil, fmt.Errorf("list tags for sample %d (%s): %w", sample.ID, sample.Path, err)
		}

		var attributes map[string]string
		stored, err := repo.GetSampleAnalysis(ctx, sample.ID)
		if err == nil {
			attributes = AttributesFromSampleAnalysis(stored)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get analysis for sample %d (%s): %w", sample.ID, sample.Path, err)
		}

		templateData := zrename.BuildTemplateData(sample, tags, attributes)
		rendered, err := zrename.RenderTemplate(templateText, templateData)
		if err != nil {
			return nil, fmt.Errorf("render template for sample %q: %w", sample.Path, err)
		}

		destination, err := zrename.ResolveTarget(sample, rendered)
		if err != nil {
			return nil, fmt.Errorf("resolve target for sample %q: %w", sample.Path, err)
		}

		ops = append(ops, zrename.RenameOp{
			Sample:      sample,
			Destination: destination,
		})
	}

	if err := zrename.ValidatePlan(ops); err != nil {
		return nil, err
	}

	return ops, nil
}

// RenameOptions controls the execution of a rename plan.
type RenameOptions struct {
	DryRun   bool
	Logger   *log.Logger
	OnRename func(op zrename.RenameOp)
}

// RenameSummary reports the result of executing a rename plan.
type RenameSummary struct {
	Renamed int
	Skipped int
}

// ExecuteRename executes a validated rename plan, moving files on disk and
// updating their sample records in the repository.
func ExecuteRename(ctx context.Context, repo *db.Repository, ops []zrename.RenameOp, opts RenameOptions) (RenameSummary, error) {
	if repo == nil {
		return RenameSummary{}, errors.New("pipeline: nil repository")
	}

	var summary RenameSummary
	for _, op := range ops {
		sample := op.Sample
		destination := op.Destination

		if opts.OnRename != nil {
			opts.OnRename(op)
		}

		if opts.DryRun || filepath.Clean(sample.Path) == destination {
			summary.Skipped++
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return summary, fmt.Errorf("create target directory: %w", err)
		}
		if err := os.Rename(sample.Path, destination); err != nil {
			return summary, fmt.Errorf("rename %q -> %q: %w", sample.Path, destination, err)
		}

		info, err := os.Stat(destination)
		if err != nil {
			return summary, fmt.Errorf("stat renamed file %q: %w", destination, err)
		}
		relativePath, err := filepath.Rel(sample.SourceRoot, destination)
		if err != nil {
			return summary, fmt.Errorf("compute relative path for %q: %w", destination, err)
		}
		if err := repo.UpdateSampleLocation(ctx, sample.ID, destination, filepath.ToSlash(relativePath), filepath.Base(destination), core.DetectFormatFromExtension(destination), info.Size(), info.ModTime()); err != nil {
			return summary, err
		}
		summary.Renamed++
	}

	return summary, nil
}
