package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrz1836/atlas/internal/constants"
	"github.com/mrz1836/atlas/internal/errors"
)

// GlobalConfigDir returns the path to the global ATLAS configuration directory.
// This is typically ~/.atlas on Unix systems.
//
// Returns an error if the home directory cannot be determined.
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "failed to get home directory")
	}
	return filepath.Join(home, constants.AtlasHome), nil
}

// ProjectConfigDir returns the relative path to the project configuration directory.
// This is always .atlas relative to the project root.
func ProjectConfigDir() string {
	return constants.AtlasHome
}

// GlobalConfigPath returns the full path to the global configuration file.
// This is typically ~/.atlas/config.yaml on Unix systems.
//
// Returns an error if the home directory cannot be determined.
func GlobalConfigPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", fmt.Errorf("get global config path: %w", err)
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// ProjectConfigPath returns the relative path to the project configuration file.
// This is always .atlas/config.yaml relative to the project root.
func ProjectConfigPath() string {
	return filepath.Join(ProjectConfigDir(), "config.yaml")
}
