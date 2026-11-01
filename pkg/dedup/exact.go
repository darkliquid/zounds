package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/darkliquid/zounds/pkg/core"
)

type KeepStrategy string

const (
	KeepFirstPath   KeepStrategy = "first-path"
	KeepOldest      KeepStrategy = "oldest"
	KeepNewest      KeepStrategy = "newest"
	KeepBestQuality KeepStrategy = "best-quality"
)

type ExactFinder struct {
	Workers int
	Logger  *log.Logger
}

type FileHash struct {
	Sample core.Sample
	SHA256 string
}

type DuplicateGroup struct {
	Hash      string
	SizeBytes int64
	Samples   []core.Sample
}

type CullAction struct {
	Hash   string
	Keep   core.Sample
	Remove []core.Sample
}

func NewExactFinder(workers int, logger ...*log.Logger) ExactFinder {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var configuredLogger *log.Logger
	if len(logger) > 0 {
		configuredLogger = logger[0]
	}
	return ExactFinder{Workers: workers, Logger: configuredLogger}
}

func (f ExactFinder) Find(ctx context.Context, samples []core.Sample) ([]DuplicateGroup, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sizeBuckets := make(map[int64][]core.Sample)
	for _, sample := range samples {
		sizeBuckets[sample.SizeBytes] = append(sizeBuckets[sample.SizeBytes], sample)
	}

	type job struct {
		sample core.Sample
	}

	jobs := make(chan job)
	results := make(chan FileHash)
	errCh := make(chan error, 1)

	workers := f.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-jobs:
					if !ok {
						return
					}
					if f.Logger != nil {
						f.Logger.Printf("hashing file %s", item.sample.Path)
					}
					hash, err := hashFile(ctx, item.sample.Path)
					if err != nil {
						if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							select {
							case errCh <- fmt.Errorf("hash %q: %w", item.sample.Path, err):
							default:
							}
							cancel()
						}
						return
					}

					select {
					case <-ctx.Done():
						return
					case results <- FileHash{Sample: item.sample, SHA256: hash}:
					}
				}
			}
		}()
	}

	var feedWG sync.WaitGroup
	feedWG.Add(1)
	go func() {
		defer feedWG.Done()
		defer close(jobs)
		for _, bucket := range sizeBuckets {
			if len(bucket) < 2 {
				continue
			}
			for _, sample := range bucket {
				select {
				case <-ctx.Done():
					return
				case jobs <- job{sample: sample}:
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	shutdown := func() {
		cancel()
		for range results {
		}
		feedWG.Wait()
	}

	grouped := map[string][]core.Sample{}
	sizeByHash := map[string]int64{}
	for {
		select {
		case <-ctx.Done():
			select {
			case err := <-errCh:
				shutdown()
				return nil, err
			default:
			}
			shutdown()
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				shutdown()
				return nil, err
			}
		case result, ok := <-results:
			if !ok {
				feedWG.Wait()
				select {
				case err := <-errCh:
					if err != nil {
						return nil, err
					}
				default:
				}
				return buildDuplicateGroups(grouped, sizeByHash), nil
			}
			grouped[result.SHA256] = append(grouped[result.SHA256], result.Sample)
			sizeByHash[result.SHA256] = result.Sample.SizeBytes
		}
	}
}

func PlanCull(groups []DuplicateGroup, strategy KeepStrategy) []CullAction {
	actions := make([]CullAction, 0, len(groups))
	for _, group := range groups {
		if len(group.Samples) < 2 {
			continue
		}

		samples := append([]core.Sample(nil), group.Samples...)
		sortSamplesForStrategy(samples, strategy)

		actions = append(actions, CullAction{
			Hash:   group.Hash,
			Keep:   samples[0],
			Remove: append([]core.Sample(nil), samples[1:]...),
		})
	}

	return actions
}

func hashFile(ctx context.Context, path string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

func buildDuplicateGroups(grouped map[string][]core.Sample, sizeByHash map[string]int64) []DuplicateGroup {
	groups := make([]DuplicateGroup, 0, len(grouped))
	for hash, samples := range grouped {
		if len(samples) < 2 {
			continue
		}

		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Path < samples[j].Path
		})

		groups = append(groups, DuplicateGroup{
			Hash:      hash,
			SizeBytes: sizeByHash[hash],
			Samples:   samples,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].SizeBytes == groups[j].SizeBytes {
			return groups[i].Hash < groups[j].Hash
		}
		return groups[i].SizeBytes < groups[j].SizeBytes
	})

	return groups
}

func sortSamplesForStrategy(samples []core.Sample, strategy KeepStrategy) {
	switch strategy {
	case KeepOldest:
		sort.Slice(samples, func(i, j int) bool {
			if samples[i].ModifiedAt.Equal(samples[j].ModifiedAt) {
				return samples[i].Path < samples[j].Path
			}
			return samples[i].ModifiedAt.Before(samples[j].ModifiedAt)
		})
	case KeepNewest:
		sort.Slice(samples, func(i, j int) bool {
			if samples[i].ModifiedAt.Equal(samples[j].ModifiedAt) {
				return samples[i].Path < samples[j].Path
			}
			return samples[i].ModifiedAt.After(samples[j].ModifiedAt)
		})
	case KeepBestQuality:
		sort.Slice(samples, func(i, j int) bool {
			qi := sampleQualityScore(samples[i])
			qj := sampleQualityScore(samples[j])
			if qi != qj {
				return qi > qj
			}
			if samples[i].SizeBytes != samples[j].SizeBytes {
				return samples[i].SizeBytes > samples[j].SizeBytes
			}
			return samples[i].Path < samples[j].Path
		})
	default:
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Path < samples[j].Path
		})
	}
}

func sampleQualityScore(s core.Sample) int {
	score := 0
	switch s.Format {
	case core.FormatWAV, core.FormatAIFF:
		score = 1000000
	case core.FormatFLAC:
		score = 900000
	case core.FormatOGG:
		score = 500000
	case core.FormatAAC, core.FormatM4A:
		score = 450000
	case core.FormatMP3:
		score = 400000
	default:
		score = 100000
	}

	if s.Metadata != nil {
		if srStr, ok := s.Metadata["sample_rate"]; ok {
			if sr, err := strconv.Atoi(srStr); err == nil && sr > 0 {
				score += sr
			}
		}
		if bdStr, ok := s.Metadata["bit_depth"]; ok {
			if bd, err := strconv.Atoi(bdStr); err == nil && bd > 0 {
				score += bd * 1000
			}
		}
		if chStr, ok := s.Metadata["channels"]; ok {
			if ch, err := strconv.Atoi(chStr); err == nil && ch > 0 {
				score += ch * 100
			}
		}
	}

	return score
}
