package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// dotenvPath returns the path to ~/.config/inflate/.env.
func dotenvPath() string {
	return filepath.Join(ConfigDir(), ".env")
}

// LoadDotenv reads ~/.config/inflate/.env and exports each KEY=VALUE pair
// via os.Setenv, but only when the env var is not already set in the real
// environment. Missing file is not an error.
func LoadDotenv() error {
	path := dotenvPath()
	parsed, err := godotenv.Read(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	for k, v := range parsed {
		if _, set := os.LookupEnv(k); set {
			continue // real env wins
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// WriteEnvVar appends or replaces the line for `key` in
// ~/.config/inflate/.env. Creates the file with mode 0600.
// Preserves any other lines (including comments and blanks) untouched.
func WriteEnvVar(key, value string) error {
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	path := dotenvPath()

	// Read existing lines, replace the matching one if present.
	var lines []string
	matched := false
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, key+"=") {
				lines = append(lines, key+"="+value)
				matched = true
			} else {
				lines = append(lines, line)
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !matched {
		lines = append(lines, key+"="+value)
	}

	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o600)
}
