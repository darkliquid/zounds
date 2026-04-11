package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

func NewRootCommand() *cobra.Command {
	cfg := DefaultConfig()

	cmd := &cobra.Command{
		Use:           "zounds",
		Short:         "Analyze and organize sound sample libraries",
		Long:          "zounds scans sample libraries, stores analysis data, and exposes CLI and web workflows for organizing audio collections.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&cfg.DatabasePath, "db", cfg.DatabasePath, "path to the SQLite database file")
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", cfg.Verbose, "enable verbose logging")
	flags.BoolVar(&cfg.DryRun, "dry-run", cfg.DryRun, "preview changes without writing files")
	flags.IntVarP(&cfg.Concurrency, "concurrency", "j", cfg.Concurrency, "maximum number of concurrent workers")

	cmd.AddCommand(
		newScanCommand(&cfg),
		newAnalyzeCommand(&cfg),
		newTagCommand(&cfg),
		newPlaceholderCommand("cluster", "Cluster related sounds"),
		newPlaceholderCommand("dedup", "Find exact or perceptual duplicates"),
		newPlaceholderCommand("convert", "Convert, resample, or normalize audio"),
		newPlaceholderCommand("rename", "Mass rename and reorganize library files"),
		newPlaceholderCommand("serve", "Run the web UI and API server"),
		newExportCommand(&cfg),
		newInfoCommand(&cfg),
		newPlayCommand(&cfg),
		newBrowseCommand(&cfg),
	)

	return cmd
}

func newPlaceholderCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s command is scaffolded but not implemented yet", cmd.Name())
		},
	}
}
