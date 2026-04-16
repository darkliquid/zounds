package commands

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/darkliquid/zounds/pkg/core"
)

type stubAnalyzer struct {
	name  string
	delay time.Duration
	run   func(context.Context, core.Sample) (core.AnalysisResult, error)
}

func (s stubAnalyzer) Name() string {
	return s.name
}

func (s stubAnalyzer) Version() string {
	return "test"
}

func (s stubAnalyzer) Analyze(ctx context.Context, sample core.Sample) (core.AnalysisResult, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.run != nil {
		return s.run(ctx, sample)
	}
	return core.AnalysisResult{
		Analyzer: s.name,
		Metrics:  map[string]float64{"ok": 1},
	}, nil
}

func TestRunAnalyzersPreservesInputOrderUnderConcurrency(t *testing.T) {
	t.Parallel()

	sample := core.Sample{ID: 1, Path: "sample.wav"}
	analyzers := []core.Analyzer{
		stubAnalyzer{name: "first", delay: 30 * time.Millisecond},
		stubAnalyzer{name: "second", delay: 5 * time.Millisecond},
		stubAnalyzer{name: "third", delay: 15 * time.Millisecond},
	}

	results, err := runAnalyzers(context.Background(), sample, analyzers, 3)
	if err != nil {
		t.Fatalf("runAnalyzers returned error: %v", err)
	}
	if len(results) != len(analyzers) {
		t.Fatalf("unexpected result count: got=%d want=%d", len(results), len(analyzers))
	}
	for i, result := range results {
		if result.Analyzer != analyzers[i].Name() {
			t.Fatalf("unexpected result order at %d: got=%q want=%q", i, result.Analyzer, analyzers[i].Name())
		}
	}
}

func TestRunAnalyzersConvertsPanicToErrorWithoutHanging(t *testing.T) {
	t.Parallel()

	sample := core.Sample{ID: 1, Path: "sample.wav"}
	analyzers := []core.Analyzer{
		stubAnalyzer{
			name: "panic-analyzer",
			run: func(context.Context, core.Sample) (core.AnalysisResult, error) {
				panic("boom")
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		_, err := runAnalyzers(context.Background(), sample, analyzers, 1)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected error from panic, got nil")
		}
		if !strings.Contains(err.Error(), "panicked") {
			t.Fatalf("expected panic error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "panic-analyzer") {
			t.Fatalf("expected analyzer name in error, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runAnalyzers did not return after panic")
	}
}

func TestRunAnalyzersWrapsAnalyzerErrors(t *testing.T) {
	t.Parallel()

	sample := core.Sample{ID: 1, Path: "sample.wav"}
	analyzers := []core.Analyzer{
		stubAnalyzer{
			name: "failing-analyzer",
			run: func(context.Context, core.Sample) (core.AnalysisResult, error) {
				return core.AnalysisResult{}, fmt.Errorf("failed")
			},
		},
	}

	_, err := runAnalyzers(context.Background(), sample, analyzers, 1)
	if err == nil {
		t.Fatalf("expected wrapped analyzer error")
	}
	if !strings.Contains(err.Error(), "failing-analyzer analysis failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
