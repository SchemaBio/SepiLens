package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrMissingAPIKeys = errors.New("auth is required but no API keys configured (set SEPILENS_API_KEYS=key:tenant)")
	ErrInvalidKeyItem = errors.New("invalid API key mapping entry, expected format key:tenant")
)

type Config struct {
	MasterKey             string
	KnowledgeDir          string
	LLMProtocol           string
	RetentionDays         int
	Principals            map[string]Principal
	Addr                  string
	DataDir               string
	DBPath                string
	AuthMode              string // "required" or "disabled"
	Simulation            bool
	AllowPrivateEndpoints bool
	LogFormat             string
	KeyToTenant           map[string]string

	// Default LLM Provider Configuration
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
}
type Principal struct {
	Tenant string `json:"tenant"`
	Actor  string `json:"actor"`
	Role   string `json:"role"`
}

func Load() (Config, error) {
	addr := getEnv("SEPILENS_ADDR", ":8080")
	dataDir := getEnv("SEPILENS_DATA_DIR", "./data")
	authMode := strings.ToLower(getEnv("SEPILENS_AUTH_MODE", "required"))
	sim := parseBool(os.Getenv("SEPILENS_SIMULATION"), false)
	allowPriv := parseBool(os.Getenv("SEPILENS_ALLOW_PRIVATE_ENDPOINTS"), false)
	logFmt := getEnv("SEPILENS_LOG_FORMAT", "json")
	retention, e := strconv.Atoi(getEnv("SEPILENS_RETENTION_DAYS", "90"))
	if e != nil || retention < 1 {
		return Config{}, errors.New("retention days must be positive")
	}
	masterKey := os.Getenv("SEPILENS_MASTER_KEY")
	if p := os.Getenv("SEPILENS_MASTER_KEY_FILE"); p != "" {
		b, e := os.ReadFile(p)
		if e != nil {
			return Config{}, errors.New("master key file unavailable")
		}
		masterKey = strings.TrimSpace(string(b))
	}
	if authMode != "required" && authMode != "disabled" {
		return Config{}, errors.New("invalid authentication mode")
	}
	principals := map[string]Principal{}
	if raw := os.Getenv("SEPILENS_PRINCIPALS"); raw != "" {
		if json.Unmarshal([]byte(raw), &principals) != nil {
			return Config{}, errors.New("invalid principal configuration")
		}
		for key, p := range principals {
			if key == "" || p.Tenant == "" || p.Actor == "" || (p.Role != "submitter" && p.Role != "reviewer" && p.Role != "admin") {
				return Config{}, errors.New("invalid principal configuration")
			}
		}
	}

	llmBaseURL := getEnv("SEPILENS_LLM_BASE_URL", getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"))
	llmAPIKey := getEnv("SEPILENS_LLM_API_KEY", getEnv("OPENAI_API_KEY", ""))
	llmModel := getEnv("SEPILENS_LLM_MODEL", getEnv("OPENAI_MODEL", "gpt-4o"))

	dbPath := filepath.Join(dataDir, "sepilens.db")

	keyMap := make(map[string]string)
	rawKeys := os.Getenv("SEPILENS_API_KEYS")
	if strings.TrimSpace(rawKeys) != "" {
		for _, item := range strings.Split(rawKeys, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.SplitN(item, ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return Config{}, ErrInvalidKeyItem
			}
			keyMap[parts[0]] = parts[1]
		}
	}

	if authMode == "required" && len(keyMap) == 0 && len(principals) == 0 {
		return Config{}, ErrMissingAPIKeys
	}

	return Config{
		MasterKey:             masterKey,
		KnowledgeDir:          getEnv("SEPILENS_KNOWLEDGE_DIR", "./knowledge/hpo"),
		LLMProtocol:           getEnv("SEPILENS_LLM_PROTOCOL", "openai_responses"),
		RetentionDays:         retention,
		Principals:            principals,
		Addr:                  addr,
		DataDir:               dataDir,
		DBPath:                dbPath,
		AuthMode:              authMode,
		Simulation:            sim,
		AllowPrivateEndpoints: allowPriv,
		LogFormat:             logFmt,
		KeyToTenant:           keyMap,
		LLMBaseURL:            llmBaseURL,
		LLMAPIKey:             llmAPIKey,
		LLMModel:              llmModel,
	}, nil
}

func (c Config) LookupTenant(key string) (string, bool) {
	if p, ok := c.Principals[key]; ok {
		return p.Tenant, true
	}
	if c.AuthMode == "disabled" {
		return "anonymous-dev", true
	}
	tenant, ok := c.KeyToTenant[key]
	return tenant, ok
}
func (c Config) LookupPrincipal(key string) (Principal, bool) {
	if c.AuthMode == "disabled" {
		return Principal{"anonymous-dev", "dev", "admin"}, true
	}
	if p, ok := c.Principals[key]; ok {
		return p, true
	}
	t, ok := c.KeyToTenant[key]
	return Principal{t, "service", "submitter"}, ok
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func parseBool(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
