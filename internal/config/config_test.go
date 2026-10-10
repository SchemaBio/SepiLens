package config_test

import (
	"errors"
	"os"
	"testing"

	"github.com/pzweuj/sepilens/internal/config"
)

func clearSepilensEnv() {
	vars := []string{
		"SEPILENS_ADDR", "SEPILENS_DATA_DIR", "SEPILENS_AUTH_MODE", "SEPILENS_SIMULATION",
		"SEPILENS_ALLOW_PRIVATE_ENDPOINTS", "SEPILENS_LOG_FORMAT", "SEPILENS_API_KEYS",
		"SEPILENS_LLM_BASE_URL", "SEPILENS_LLM_API_KEY", "SEPILENS_LLM_MODEL",
		"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}

func TestConfig_Load(t *testing.T) {
	clearSepilensEnv()

	// Default auth mode is required, missing keys should fail
	_, err := config.Load()
	if !errors.Is(err, config.ErrMissingAPIKeys) {
		t.Fatalf("expected ErrMissingAPIKeys, got %v", err)
	}

	// Disabled auth mode allows empty keys
	os.Setenv("SEPILENS_AUTH_MODE", "disabled")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error with auth disabled: %v", err)
	}
	if tenant, ok := cfg.LookupTenant("any"); !ok || tenant != "anonymous-dev" {
		t.Fatalf("expected anonymous tenant, got %q (ok=%v)", tenant, ok)
	}

	// Valid key mapping
	os.Setenv("SEPILENS_AUTH_MODE", "required")
	os.Setenv("SEPILENS_API_KEYS", "key1:tenantA,key2:tenantB")
	cfg2, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error with valid keys: %v", err)
	}
	tenantA, okA := cfg2.LookupTenant("key1")
	if !okA || tenantA != "tenantA" {
		t.Fatalf("expected tenantA, got %q (ok=%v)", tenantA, okA)
	}
	_, okMissing := cfg2.LookupTenant("nonexistent")
	if okMissing {
		t.Fatalf("expected false for nonexistent key")
	}

	// Malformed key entry
	os.Setenv("SEPILENS_API_KEYS", "badkey")
	_, errBad := config.Load()
	if !errors.Is(errBad, config.ErrInvalidKeyItem) {
		t.Fatalf("expected ErrInvalidKeyItem, got %v", errBad)
	}

	// LLM config parsing
	os.Unsetenv("SEPILENS_API_KEYS")
	os.Setenv("SEPILENS_AUTH_MODE", "disabled")
	os.Setenv("SEPILENS_LLM_BASE_URL", "https://api.deepseek.com/v1")
	os.Setenv("SEPILENS_LLM_API_KEY", "sk-test-key")
	os.Setenv("SEPILENS_LLM_MODEL", "deepseek-chat")
	cfgLLM, errLLM := config.Load()
	if errLLM != nil {
		t.Fatalf("unexpected error parsing LLM config: %v", errLLM)
	}
	if cfgLLM.LLMBaseURL != "https://api.deepseek.com/v1" || cfgLLM.LLMAPIKey != "sk-test-key" || cfgLLM.LLMModel != "deepseek-chat" {
		t.Fatalf("mismatched LLM config parsed: %+v", cfgLLM)
	}
}
