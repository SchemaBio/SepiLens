package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// AutoLoadDotEnv searches for .env in the binary's directory and current working directory,
// loading any variables that are not yet set in the environment.
func AutoLoadDotEnv() []string {
	var loaded []string
	var candidates []string

	// 1. Directory of the running binary
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, ".env"))
	}

	// 2. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env"))
	}

	seen := make(map[string]bool)
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true

		if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
			if err := LoadDotEnvFile(abs); err == nil {
				loaded = append(loaded, abs)
			}
		}
	}
	return loaded
}

// LoadDotEnvFile parses a .env formatted file and sets any environment variables
// that are currently unset.
func LoadDotEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}

		val := strings.TrimSpace(parts[1])
		// Handle quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Only populate if not already set in process environment
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
	return scanner.Err()
}
