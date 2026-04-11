package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
	zconvert "github.com/darkliquid/zounds/pkg/convert"
	"github.com/darkliquid/zounds/pkg/core"
)

func newConvertCommand(cfg *Config) *cobra.Command {
	var (
		outputPath    string
		formatName    string
		sampleRate    int
		channels      int
		normalizeMode string
		targetDBFS    float64
		allowClipping bool
	)

	cmd := &cobra.Command{
		Use:   "convert <source>",
		Short: "Convert, resample, or normalize audio",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourcePath := args[0]

			targetFormat, err := parseTargetFormat(formatName)
			if err != nil {
				return err
			}

			targetPath, err := resolveConvertTarget(sourcePath, outputPath, targetFormat)
			if err != nil {
				return err
			}
			if sourcePath == targetPath {
				return fmt.Errorf("source and target paths must differ")
			}

			if cfg.DryRun {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "convert %s -> %s", sourcePath, targetPath)
				if err != nil {
					return err
				}
				if sampleRate > 0 {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), " samplerate=%d", sampleRate); err != nil {
						return err
					}
				}
				if channels > 0 {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), " channels=%d", channels); err != nil {
						return err
					}
				}
				if normalizeMode != "" {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), " normalize=%s@%.2fdbfs", normalizeMode, targetDBFS); err != nil {
						return err
					}
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout())
				return err
			}

			registry, err := codecs.NewRegistry()
			if err != nil {
				return err
			}

			result, err := zaudio.DecodeFile(cmd.Context(), registry, sourcePath)
			if err != nil {
				return err
			}

			buffer, err := applyConversions(result.Buffer, sampleRate, channels, normalizeMode, targetDBFS, allowClipping)
			if err != nil {
				return err
			}

			return zaudio.EncodeFile(cmd.Context(), registry, targetPath, buffer)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&outputPath, "output", "o", "", "target output file path")
	flags.StringVar(&formatName, "format", "", "target format (wav or aiff)")
	flags.IntVar(&sampleRate, "samplerate", 0, "target sample rate")
	flags.IntVar(&channels, "channels", 0, "target channel count")
	flags.StringVar(&normalizeMode, "normalize", "", "normalize mode: peak, rms, or lufs")
	flags.Float64Var(&targetDBFS, "target-dbfs", -1, "normalization target in dBFS")
	flags.BoolVar(&allowClipping, "allow-clipping", false, "allow normalization to clip instead of erroring")

	return cmd
}

func parseTargetFormat(name string) (core.AudioFormat, error) {
	if strings.TrimSpace(name) == "" {
		return core.FormatUnknown, nil
	}

	format := core.DetectFormatFromExtension("x." + strings.TrimSpace(name))
	if format == core.FormatUnknown {
		return core.FormatUnknown, fmt.Errorf("unsupported target format %q", name)
	}

	return format, nil
}

func resolveConvertTarget(sourcePath, outputPath string, targetFormat core.AudioFormat) (string, error) {
	if outputPath == "" {
		if targetFormat == core.FormatUnknown {
			return "", fmt.Errorf("use --output or --format to choose a target file")
		}
		outputPath = strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + "." + string(targetFormat)
	}

	if targetFormat == core.FormatUnknown {
		targetFormat = core.DetectFormatFromExtension(outputPath)
		if targetFormat == core.FormatUnknown {
			return "", fmt.Errorf("could not infer target format from %q", outputPath)
		}
		return outputPath, nil
	}

	if filepath.Ext(outputPath) == "" {
		return outputPath + "." + string(targetFormat), nil
	}

	outputFormat := core.DetectFormatFromExtension(outputPath)
	if outputFormat != targetFormat {
		return "", fmt.Errorf("output path format %q does not match --format %q", outputFormat, targetFormat)
	}

	return outputPath, nil
}

func applyConversions(buffer zaudio.PCMBuffer, sampleRate, channels int, normalizeMode string, targetDBFS float64, allowClipping bool) (zaudio.PCMBuffer, error) {
	var err error

	if sampleRate > 0 && sampleRate != buffer.SampleRate {
		buffer, err = zconvert.ResampleLinear(buffer, zconvert.ResampleOptions{TargetSampleRate: sampleRate})
		if err != nil {
			return zaudio.PCMBuffer{}, err
		}
	}

	if channels > 0 && channels != buffer.Channels {
		buffer, err = zconvert.ConvertChannels(buffer, zconvert.ChannelOptions{TargetChannels: channels})
		if err != nil {
			return zaudio.PCMBuffer{}, err
		}
	}

	if strings.TrimSpace(normalizeMode) != "" {
		mode := zconvert.NormalizeMode(strings.TrimSpace(strings.ToLower(normalizeMode)))
		buffer, err = zconvert.Normalize(buffer, zconvert.NormalizeOptions{
			Mode:          mode,
			TargetDBFS:    targetDBFS,
			AllowClipping: allowClipping,
		})
		if err != nil {
			return zaudio.PCMBuffer{}, err
		}
	}

	return buffer, nil
}
