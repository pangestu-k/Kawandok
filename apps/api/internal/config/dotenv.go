package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv parses a KEY=VALUE file and sets variables that are not already
// present in the environment (existing environment wins). Lines starting with
// '#' and blank lines are ignored; surrounding single or double quotes on
// values are stripped.
func LoadDotEnv(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("dotenv: %w", err)
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("dotenv: set %s: %w", key, err)
			}
		}
	}
	return nil
}

// FindDotEnv walks up from startDir looking for a `.env` file and returns its
// path. It stops after 8 levels or at the filesystem root.
func FindDotEnv(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, ".env")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
	return "", false
}
