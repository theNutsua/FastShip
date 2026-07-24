package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultFilename is the config file FastShip looks for.
const DefaultFilename = "fastship.yaml"

// Load reads a fastship.yaml from disk, applies defaults, and validates it.
// path may be a file or a directory. If it is a directory, Load looks
// for fastship.yaml inside it. An empty path means the current directory.
// The returned Config is ready to hand to any subsystem — defaults are
// filled and validation has passed.
func Load(path string) (*Config, error) {
	file, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(file)
	if err != nil {
		// Distinguish "no config here" from "config exists but unreadable",
		// because the fix for each is completely different.
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no %s found at %s", DefaultFilename, file)
		}
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}

	return Parse(data)
}

// Parse turns raw YAML bytes into a validated Config.
// Separate from Load so tests can exercise parsing without touching
// the filesystem.
func Parse(data []byte) (*Config, error) {
	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing fastship.yaml: %w", err)
	}

	// Defaults before validation: validation should judge the config as
	// FastShip will actually run it, not as the engineer typed it.
	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// resolvePath turns a user-supplied path into a concrete file path.
func resolvePath(path string) (string, error) {
	if path == "" {
		path = "."
	}

	info, err := os.Stat(path)
	if err != nil {
		// Path does not exist. If it looks like a directory the user meant,
		// still point at the expected file so the error names fastship.yaml.
		return filepath.Join(path, DefaultFilename), nil
	}

	if info.IsDir() {
		return filepath.Join(path, DefaultFilename), nil
	}

	return path, nil
}
