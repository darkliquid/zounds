package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

var defaultFormats = map[string]core.AudioFormat{
	".wav":  core.FormatWAV,
	".aif":  core.FormatAIFF,
	".aiff": core.FormatAIFF,
	".mp3":  core.FormatMP3,
	".flac": core.FormatFLAC,
	".ogg":  core.FormatOGG,
	".oga":  core.FormatOGG,
	".aac":  core.FormatAAC,
	".m4a":  core.FormatM4A,
}

type Options struct {
	Workers       int
	FollowSymlink bool
	IncludeHidden bool
	Formats       map[string]core.AudioFormat
	Logger        *log.Logger
}

type Scanner struct {
	workers       int
	followSymlink bool
	includeHidden bool
	formats       map[string]core.AudioFormat
	logger        *log.Logger
}

type candidate struct {
	root string
	path string
}

func New(opts Options) *Scanner {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	formats := opts.Formats
	if len(formats) == 0 {
		formats = make(map[string]core.AudioFormat, len(defaultFormats))
		for ext, format := range defaultFormats {
			formats[ext] = format
		}
	}

	return &Scanner{
		workers:       workers,
		followSymlink: opts.FollowSymlink,
		includeHidden: opts.IncludeHidden,
		formats:       formats,
		logger:        opts.Logger,
	}
}

func (s *Scanner) Scan(ctx context.Context, roots ...string) ([]core.Sample, error) {
	if len(roots) == 0 {
		return nil, errors.New("scan requires at least one root")
	}

	s.logf("starting scan with %d root(s) and %d worker(s)", len(roots), s.workers)

	candidates := make(chan candidate)
	results := make(chan core.Sample)
	errCh := make(chan error, 1)

	var workerWG sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for item := range candidates {
				sample, err := s.statCandidate(item.root, item.path)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}

				select {
				case <-ctx.Done():
					return
				case results <- sample:
				}
			}
		}()
	}

	go func() {
		workerWG.Wait()
		close(results)
	}()

	go func() {
		defer close(candidates)
		for _, root := range roots {
			if err := s.walkRoot(ctx, root, candidates); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}()

	var samples []core.Sample
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		case sample, ok := <-results:
			if !ok {
				sort.Slice(samples, func(i, j int) bool {
					return samples[i].Path < samples[j].Path
				})
				s.logf("scan complete: discovered %d audio file(s)", len(samples))
				return samples, nil
			}
			samples = append(samples, sample)
		}
	}
}

func SupportedExtensions() []string {
	keys := make([]string, 0, len(defaultFormats))
	for ext := range defaultFormats {
		keys = append(keys, ext)
	}
	sort.Strings(keys)
	return keys
}

func (s *Scanner) walkRoot(ctx context.Context, root string, out chan<- candidate) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root %q: %w", root, err)
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return fmt.Errorf("stat root %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("scan root %q is not a directory", absRoot)
	}

	s.logf("walking root %s", absRoot)

	return filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if path != absRoot && !s.includeHidden && isHidden(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if entry.IsDir() {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 && !s.followSymlink {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, ok := s.formats[ext]; !ok {
			return nil
		}

		s.logf("scanning file %s", path)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- candidate{root: absRoot, path: path}:
			return nil
		}
	})
}

func (s *Scanner) statCandidate(root, path string) (core.Sample, error) {
	info, err := os.Stat(path)
	if err != nil {
		return core.Sample{}, fmt.Errorf("stat candidate %q: %w", path, err)
	}

	if info.IsDir() {
		return core.Sample{}, fmt.Errorf("candidate %q unexpectedly resolved to directory", path)
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return core.Sample{}, fmt.Errorf("derive relative path for %q: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	format, ok := s.formats[ext]
	if !ok {
		format = core.DetectFormatFromExtension(path)
	}

	return core.Sample{
		SourceRoot:   root,
		Path:         path,
		RelativePath: filepath.ToSlash(relPath),
		FileName:     info.Name(),
		Extension:    strings.TrimPrefix(ext, "."),
		Format:       format,
		SizeBytes:    info.Size(),
		ModifiedAt:   info.ModTime().UTC(),
		ScannedAt:    time.Now().UTC(),
	}, nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func (s *Scanner) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
