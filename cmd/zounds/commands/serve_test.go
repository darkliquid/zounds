package commands

import (
	"context"
	"errors"
	"testing"
)

func TestRunServeRejectsInvalidPort(t *testing.T) {
	t.Parallel()

	err := runServe(context.Background(), &Config{DatabasePath: ":memory:"}, "127.0.0.1", 0, nil)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestRunServeReturnsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runServe(ctx, &Config{DatabasePath: ":memory:"}, "127.0.0.1", 18080, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
