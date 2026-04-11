package commands

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	"github.com/darkliquid/zounds/pkg/db"
)

type browseResult struct {
	Sample core.Sample
	Tags   []core.Tag
}

func newBrowseCommand(cfg *Config) *cobra.Command {
	var (
		requiredTags []string
		showTags     bool
		limit        int
		preview      bool
		volume       float64
		noBlock      bool
	)

	cmd := &cobra.Command{
		Use:   "browse [filter]",
		Short: "Browse indexed samples with filterable output",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, closer, err := openRepository(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			query := ""
			if len(args) == 1 {
				query = args[0]
			}

			results, err := browseMatches(cmd.Context(), repo, query, requiredTags)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no samples matched the current browse filter")
				return err
			}

			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}

			if preview {
				target := results[0].Sample.Path
				if cfg.DryRun {
					_, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved browse preview target: %s\n", target)
					return err
				}
				return playTarget(cmd.Context(), cmd.OutOrStdout(), target, volume, noBlock)
			}

			for idx, result := range results {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d.\t%s", idx+1, result.Sample.Path); err != nil {
					return err
				}
				if showTags {
					names := make([]string, 0, len(result.Tags))
					for _, tag := range result.Tags {
						names = append(names, tag.NormalizedName)
					}
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\t%s", strings.Join(names, ", ")); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringSliceVar(&requiredTags, "tag", nil, "require one or more tags on matching samples")
	flags.BoolVar(&showTags, "tags", false, "include tags in browse output")
	flags.IntVar(&limit, "limit", 25, "maximum number of matches to display")
	flags.BoolVar(&preview, "play", false, "play the first matching sample")
	flags.Float64Var(&volume, "volume", 1.0, "playback volume multiplier")
	flags.BoolVar(&noBlock, "no-block", false, "return immediately after starting playback")

	return cmd
}

func browseMatches(ctx context.Context, repo *db.Repository, query string, requiredTags []string) ([]browseResult, error) {
	samples, err := repo.ListSamples(ctx)
	if err != nil {
		return nil, err
	}

	normalizedQuery := core.NormalizeTagName(query)
	normalizedRequired := make([]string, 0, len(requiredTags))
	for _, tag := range requiredTags {
		normalizedRequired = append(normalizedRequired, core.NormalizeTagName(tag))
	}

	results := make([]browseResult, 0, len(samples))
	for _, sample := range samples {
		tags, err := repo.ListTagsForSample(ctx, sample.ID)
		if err != nil {
			return nil, err
		}
		result := browseResult{Sample: sample, Tags: tags}
		if browseResultMatches(result, normalizedQuery, normalizedRequired) {
			results = append(results, result)
		}
	}

	return results, nil
}

func browseResultMatches(result browseResult, query string, requiredTags []string) bool {
	tagNames := make([]string, 0, len(result.Tags))
	for _, tag := range result.Tags {
		tagNames = append(tagNames, tag.NormalizedName)
	}

	for _, required := range requiredTags {
		if !slices.Contains(tagNames, required) {
			return false
		}
	}

	if strings.TrimSpace(query) == "" {
		return true
	}

	haystacks := []string{
		core.NormalizeTagName(result.Sample.Path),
		core.NormalizeTagName(result.Sample.RelativePath),
		core.NormalizeTagName(result.Sample.FileName),
	}
	haystacks = append(haystacks, tagNames...)

	for _, haystack := range haystacks {
		if strings.Contains(haystack, query) {
			return true
		}
	}

	return false
}
