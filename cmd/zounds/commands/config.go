package commands

import "runtime"

type Config struct {
	DatabasePath string
	Verbose      bool
	DryRun       bool
	Concurrency  int
}

func DefaultConfig() Config {
	return Config{
		DatabasePath: "zounds.db",
		Concurrency:  runtime.NumCPU(),
	}
}
