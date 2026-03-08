package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NotesDir string `yaml:"notes_dir"`
	DBPath   string `yaml:"db_path"`
	Editor   string `yaml:"editor"`
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		NotesDir: filepath.Join(home, "moss", "notes"),
		DBPath:   filepath.Join(home, "moss", "moss.db"),
		Editor:   getEditor(),
	}
}

func getEditor() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func Load() (Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	configPath := filepath.Join(home, "moss", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Config file doesn't exist, use defaults
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	// Fill in defaults for empty fields
	def := DefaultConfig()
	if cfg.NotesDir == "" {
		cfg.NotesDir = def.NotesDir
	}
	if cfg.DBPath == "" {
		cfg.DBPath = def.DBPath
	}
	if cfg.Editor == "" {
		cfg.Editor = def.Editor
	}

	return cfg, nil
}
