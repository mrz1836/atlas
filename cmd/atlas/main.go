// Package main provides the entry point for the atlas CLI.
package main

import (
	"context"
	"os"

	"github.com/mrz1836/atlas/internal/cli"
)

// Build info variables set via ldflags during build.
// Example: go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse HEAD)"
//
//nolint:gochecknoglobals // Required for ldflags injection at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Ensure log file is properly flushed and closed on exit
	defer cli.CloseLogFile()

	ctx := context.Background()
	err := cli.Execute(ctx, cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		os.Exit(cli.ExitCodeForError(err))
	}
}
