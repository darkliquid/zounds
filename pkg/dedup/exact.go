package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/darkliquid/zounds/pkg/core"
)

type KeepStrategy string

const (
	KeepFirstPath KeepStrategy = "first-path"
	KeepOldest    KeepStrategy = "oldest"
	KeepNewest    KeepStrategy = "newest"
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

	var wg sync.WaitGroup
	for i := 0; i < f.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				if f.Logger != nil {
					f.Logger.Printf("hashing file %s", item.sample.Path)
				}
				hash, err := hashFile(ctx, item.sample.Path)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("hash %q: %w", item.sample.Path, err):
					default:
					}
					return
				}

				select {
				case <-ctx.Done():
					return
				case results <- FileHash{Sample: item.sample, SHA256: hash}:
				}
			}
		}()
	}

	go func() {
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

	grouped := map[string][]core.Sample{}
	sizeByHash := map[string]int64{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
		case result, ok := <-results:
			if !ok {
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
	default:
		sort.Slice(samples, func(i, j int) bool {
			return samples[i].Path < samples[j].Path
		})
	}
}
