package commands

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/cluster"
	"github.com/darkliquid/zounds/pkg/core"
)

type similarMatch struct {
	Sample      core.Sample
	Reference   core.Sample
	Similarity  float64
	Distance    float64
	ReferenceID int64
}

type similarReference struct {
	Sample core.Sample
	Vector core.FeatureVector
}

func newSimilarCommand(cfg *Config) *cobra.Command {
	var (
		limit     int
		threshold float64
		order     string
	)

	cmd := &cobra.Command{
		Use:   "similar <sample-ref> [sample-ref...]",
		Short: "List indexed sounds similar to one or more references",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return fmt.Errorf("limit must be greater than or equal to 0")
			}
			if threshold < -1 || threshold > 1 {
				return fmt.Errorf("threshold must be between -1 and 1")
			}
			if order != "asc" && order != "desc" {
				return fmt.Errorf("unsupported order %q", order)
			}

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
			if len(samples) == 0 {
				return fmt.Errorf("no indexed samples found")
			}

			vectors, err := repo.ListFeatureVectors(ctx, "analysis")
			if err != nil {
				return err
			}
			if len(vectors) == 0 {
				return fmt.Errorf("no analysis feature vectors found; run zounds analyze first")
			}

			references, err := resolveSimilarReferences(args, samples, vectors)
			if err != nil {
				return err
			}

			matches, err := findSimilarMatches(references, samples, vectors, threshold, order)
			if err != nil {
				return err
			}
			if limit > 0 && len(matches) > limit {
				matches = matches[:limit]
			}
			if len(matches) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no similar samples matched the current threshold")
				return err
			}

			for _, match := range matches {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%.4f\t%s\t%s\n",
					match.Similarity,
					match.Reference.Path,
					match.Sample.Path,
				); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.IntVar(&limit, "limit", 25, "maximum number of similar samples to display (0 for unlimited)")
	flags.Float64Var(&threshold, "threshold", 0, "minimum cosine similarity required")
	flags.StringVar(&order, "order", "desc", "sort order for similarity: asc or desc")

	return cmd
}

func resolveSimilarReferences(args []string, samples []core.Sample, vectors []core.FeatureVector) ([]similarReference, error) {
	sampleByID := make(map[int64]core.Sample, len(samples))
	sampleByPath := make(map[string]core.Sample, len(samples))
	samplesByRelativePath := make(map[string][]core.Sample)
	samplesByFileName := make(map[string][]core.Sample)
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
		sampleByPath[sample.Path] = sample
		samplesByRelativePath[sample.RelativePath] = append(samplesByRelativePath[sample.RelativePath], sample)
		samplesByFileName[sample.FileName] = append(samplesByFileName[sample.FileName], sample)
	}

	vectorBySampleID := make(map[int64]core.FeatureVector, len(vectors))
	for _, vector := range vectors {
		vectorBySampleID[vector.SampleID] = vector
	}

	references := make([]similarReference, 0, len(args))
	seen := make(map[int64]struct{}, len(args))
	for _, arg := range args {
		sample, err := resolveSimilarReference(arg, sampleByID, sampleByPath, samplesByRelativePath, samplesByFileName)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[sample.ID]; ok {
			continue
		}

		vector, ok := vectorBySampleID[sample.ID]
		if !ok {
			return nil, fmt.Errorf("sample %q has no analysis feature vector; run zounds analyze first", sample.Path)
		}

		references = append(references, similarReference{
			Sample: sample,
			Vector: vector,
		})
		seen[sample.ID] = struct{}{}
	}

	return references, nil
}

func resolveSimilarReference(
	arg string,
	sampleByID map[int64]core.Sample,
	sampleByPath map[string]core.Sample,
	samplesByRelativePath map[string][]core.Sample,
	samplesByFileName map[string][]core.Sample,
) (core.Sample, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil {
		sample, ok := sampleByID[id]
		if !ok {
			return core.Sample{}, fmt.Errorf("sample id %d is not indexed", id)
		}
		return sample, nil
	}

	if sample, ok := sampleByPath[arg]; ok {
		return sample, nil
	}

	if sample, err := resolveUniqueSampleReference(arg, samplesByRelativePath, "relative path"); err == nil {
		return sample, nil
	} else if err != nil && !isReferenceNotFound(err) {
		return core.Sample{}, err
	}

	if sample, err := resolveUniqueSampleReference(arg, samplesByFileName, "file name"); err == nil {
		return sample, nil
	} else if err != nil && !isReferenceNotFound(err) {
		return core.Sample{}, err
	}

	return core.Sample{}, fmt.Errorf("no indexed sample matched %q", arg)
}

func resolveUniqueSampleReference(arg string, index map[string][]core.Sample, label string) (core.Sample, error) {
	matches := index[arg]
	switch len(matches) {
	case 0:
		return core.Sample{}, errReferenceNotFound
	case 1:
		return matches[0], nil
	default:
		return core.Sample{}, fmt.Errorf("reference %q matched multiple samples by %s; use an id or full path", arg, label)
	}
}

var errReferenceNotFound = fmt.Errorf("reference not found")

func isReferenceNotFound(err error) bool {
	return err == errReferenceNotFound
}

func findSimilarMatches(
	references []similarReference,
	samples []core.Sample,
	vectors []core.FeatureVector,
	threshold float64,
	order string,
) ([]similarMatch, error) {
	sampleByID := make(map[int64]core.Sample, len(samples))
	for _, sample := range samples {
		sampleByID[sample.ID] = sample
	}

	referenceIDs := make(map[int64]struct{}, len(references))
	for _, reference := range references {
		referenceIDs[reference.Sample.ID] = struct{}{}
	}

	matches := make([]similarMatch, 0, len(vectors))
	for _, candidate := range vectors {
		if _, ok := referenceIDs[candidate.SampleID]; ok {
			continue
		}

		sample, ok := sampleByID[candidate.SampleID]
		if !ok {
			return nil, fmt.Errorf("sample id %d referenced by analysis vector is not indexed", candidate.SampleID)
		}

		bestSimilarity := math.Inf(-1)
		var bestReference core.Sample
		for _, reference := range references {
			similarity, err := cluster.CosineSimilarity(reference.Vector.Values, candidate.Values)
			if err != nil {
				return nil, fmt.Errorf("compare %q to %q: %w", reference.Sample.Path, sample.Path, err)
			}
			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestReference = reference.Sample
			}
		}

		if bestSimilarity < threshold {
			continue
		}

		matches = append(matches, similarMatch{
			Sample:      sample,
			Reference:   bestReference,
			ReferenceID: bestReference.ID,
			Similarity:  bestSimilarity,
			Distance:    1 - bestSimilarity,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Similarity == matches[j].Similarity {
			if matches[i].Sample.Path == matches[j].Sample.Path {
				return matches[i].Reference.Path < matches[j].Reference.Path
			}
			return matches[i].Sample.Path < matches[j].Sample.Path
		}
		if order == "asc" {
			return matches[i].Similarity < matches[j].Similarity
		}
		return matches[i].Similarity > matches[j].Similarity
	})

	return matches, nil
}
