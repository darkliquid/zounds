package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
)

func newPlayCommand(cfg *Config) *cobra.Command {
	var (
		volume  float64
		noBlock bool
	)

	cmd := &cobra.Command{
		Use:   "play <file-or-tag>",
		Short: "Play a sample or search result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := resolvePlaybackTarget(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}

			if cfg.DryRun {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved playback target: %s\n", target)
				return err
			}

			return playTarget(cmd.Context(), cmd.OutOrStdout(), target, volume, noBlock)
		},
	}

	flags := cmd.Flags()
	flags.Float64Var(&volume, "volume", 1.0, "playback volume multiplier")
	flags.BoolVar(&noBlock, "no-block", false, "return immediately after starting playback")

	return cmd
}

func resolvePlaybackTarget(ctx context.Context, cfg *Config, input string) (string, error) {
	if _, err := os.Stat(input); err == nil {
		return input, nil
	}

	repo, closer, err := openRepository(ctx, cfg)
	if err != nil {
		return "", err
	}
	defer closer.Close()

	samples, err := repo.FindSamplesByTag(ctx, input)
	if err != nil {
		return "", err
	}
	if len(samples) == 0 {
		return "", fmt.Errorf("no file or indexed tag match found for %q", input)
	}

	return samples[0].Path, nil
}

func playTarget(ctx context.Context, out io.Writer, target string, volume float64, noBlock bool) error {
	registry, err := codecs.NewRegistry()
	if err != nil {
		return err
	}

	result, err := zaudio.DecodeFile(ctx, registry, target)
	if err != nil {
		return err
	}

	player, err := zaudio.NewPlayback(zaudio.PlaybackOptions{Registry: registry})
	if err != nil {
		return err
	}
	if err := player.PlayBuffer(ctx, result.Buffer); err != nil {
		return err
	}
	player.SetVolume(volume)

	if noBlock {
		_, err := fmt.Fprintf(out, "playing %s\n", target)
		return err
	}

	waitForPlayback(result.Buffer.Duration())
	return player.Stop()
}

func waitForPlayback(duration time.Duration) {
	if duration <= 0 {
		duration = 250 * time.Millisecond
	}
	time.Sleep(duration + 100*time.Millisecond)
}
