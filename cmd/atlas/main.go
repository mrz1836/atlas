// Package main provides the entry point for the atlas CLI.
package main

import (
	"context"
	"os"

	"github.com/mrz1836/atlas/internal/cli"
)

func main() {
	ctx := context.Background()
	if err := cli.Execute(ctx); err != nil {
		os.Exit(1)
	}
}
