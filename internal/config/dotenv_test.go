package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/config"
)

func TestLoadDotEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `
# Comment line
SEPILENS_TEST_DOTENV_KEY=my_secret_token
export SEPILENS_TEST_DOTENV_QUOTED="quoted_value"
SEPILENS_TEST_DOTENV_SINGLE='single_value'
SEPILENS_TEST_EXISTING=new_value
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	// Set an existing variable to verify non-overwrite behavior
	_ = os.Setenv("SEPILENS_TEST_EXISTING", "original_value")
	defer os.Unsetenv("SEPILENS_TEST_EXISTING")
	defer os.Unsetenv("SEPILENS_TEST_DOTENV_KEY")
	defer os.Unsetenv("SEPILENS_TEST_DOTENV_QUOTED")
	defer os.Unsetenv("SEPILENS_TEST_DOTENV_SINGLE")

	if err := config.LoadDotEnvFile(envPath); err != nil {
		t.Fatalf("LoadDotEnvFile failed: %v", err)
	}

	if val := os.Getenv("SEPILENS_TEST_DOTENV_KEY"); val != "my_secret_token" {
		t.Fatalf("expected my_secret_token, got %q", val)
	}
	if val := os.Getenv("SEPILENS_TEST_DOTENV_QUOTED"); val != "quoted_value" {
		t.Fatalf("expected quoted_value, got %q", val)
	}
	if val := os.Getenv("SEPILENS_TEST_DOTENV_SINGLE"); val != "single_value" {
		t.Fatalf("expected single_value, got %q", val)
	}
	// Existing value should remain untouched
	if val := os.Getenv("SEPILENS_TEST_EXISTING"); val != "original_value" {
		t.Fatalf("expected original_value preserved, got %q", val)
	}
}

func TestAutoLoadDotEnv_GracefulWhenMissing(t *testing.T) {
	// Should not panic or error even if no .env exists
	loaded := config.AutoLoadDotEnv()
	_ = loaded
}
