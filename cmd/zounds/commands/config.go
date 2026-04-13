package commands

import (
	"runtime"

	"github.com/darkliquid/zounds/pkg/db"
)

type Config struct {
	DatabasePath string
	Verbose      bool
	DryRun       bool
	Concurrency  int
}

func DefaultConfig() Config {
	return Config{
		DatabasePath: db.DefaultPath(),
		Concurrency:  runtime.NumCPU(),
	}
}
