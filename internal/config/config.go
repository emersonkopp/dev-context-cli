// Package config manages the CLI configuration persisted in ~/.dev-context/config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName  = ".dev-context"
	fileName = "config.json"
)

// Config holds all CLI settings.
type Config struct {
	// RepoPath is the local path to the dev-context monorepo.
	RepoPath string `json:"repo_path"`
	// RepoURL is the remote GitHub URL of the monorepo.
	RepoURL string `json:"repo_url"`
}

// DefaultRepoURL is the canonical remote for the monorepo.
const DefaultRepoURL = "https://github.com/emersonkopp/dev-context"

// Dir returns the path to ~/.dev-context/.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, dirName), nil
}

// Path returns the full path to the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads the config file. Returns ErrNotFound if the file does not exist yet.
var ErrNotFound = errors.New("config not found — run 'dctx config init' first")

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save persists the config, creating the directory if necessary.
func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	p := filepath.Join(dir, fileName)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Init creates a default config if one does not already exist.
// repoPath is the local clone path; if empty, defaults to ~/git/dev-context.
func Init(repoPath string) (*Config, error) {
	if repoPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		repoPath = filepath.Join(home, "git", "dev-context")
	}
	cfg := &Config{
		RepoPath: repoPath,
		RepoURL:  DefaultRepoURL,
	}
	if err := Save(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
