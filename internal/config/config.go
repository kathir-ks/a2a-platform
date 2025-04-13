// internal/config/config.go
package config

import (
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Config holds all configuration for the application.
type Config struct {
	HTTPPort              int           `mapstructure:"HTTP_PORT"`
	LogLevel              string        `mapstructure:"LOG_LEVEL"`
	DatabaseURL           string        `mapstructure:"DATABASE_URL"` // Example: postgres://user:pass@host:port/db?sslmode=disable
	AgentCallTimeout      time.Duration `mapstructure:"AGENT_CALL_TIMEOUT_SECONDS"`
	// Add other config fields as needed:
	// JWTSecret string `mapstructure:"JWT_SECRET"`
	// MaxHistoryLength int `mapstructure:"MAX_HISTORY_LENGTH"`
	
	// --- LLM Configuration ---
	// Use map for flexibility, or specific fields if only supporting one/two providers
	LLMProviders map[string]LLMProviderConfig `mapstructure:"LLM_PROVIDERS"`
	DefaultLLMModel string                    `mapstructure:"DEFAULT_LLM_MODEL"` // e.g., "openai:gpt-4o"

}


// LLMProviderConfig holds settings for a specific LLM provider.
type LLMProviderConfig struct {
	APIKey string `mapstructure:"API_KEY"`
	// Add other provider-specific configs like BaseURL, OrgID etc. if needed
}

// LoadConfig reads configuration from file and environment variables.
func LoadConfig(configPaths ...string) (*Config, error) {
	v := viper.New()

	// --- Set Defaults ---
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("DATABASE_URL", "sqlite:a2a_platform.db") // Default to SQLite file in current dir
	v.SetDefault("AGENT_CALL_TIMEOUT_SECONDS", 15)         // Default timeout for calling agents
	v.SetDefault("DEFAULT_LLM_MODEL", "openai:gpt-4o") // Example default using provider prefix
    v.SetDefault("LLM_PROVIDERS", map[string]LLMProviderConfig{}) // Default to empty map

	// --- Configure Viper ---
	v.SetConfigName("config") // Name of config file (without extension)
	v.SetConfigType("yaml")   // REQUIRED if the config file does not have the extension in the name

	// Add paths to search for the config file
	v.AddConfigPath(".") // Look in the current directory
	for _, path := range configPaths {
		if path != "" {
			v.AddConfigPath(path)
		}
	}
	// Optional: Add more standard paths like /etc/appname/ or $HOME/.appname
	// v.AddConfigPath("/etc/a2a-platform/")
	// v.AddConfigPath("$HOME/.a2a-platform")

	// --- Environment Variables ---
	v.AutomaticEnv()                                         // Read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))       // Replace dots with underscores in env var names (e.g., http.port -> HTTP_PORT)
	v.AllowEmptyEnv(true)                                    // Allow empty env vars (though defaults usually handle this)

	// --- Read Config File ---
	err := v.ReadInConfig()
	if err != nil {
		// If the config file is not found, it's okay if we have defaults/env vars,
		// but log it just in case. For other errors, return them.
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warn("Config file not found, using defaults and environment variables.")
		} else {
			log.Errorf("Error reading config file: %v", err)
			return nil, err
		}
	} else {
		log.Infof("Using config file: %s", v.ConfigFileUsed())
	}

	// --- Unmarshal into Struct ---
	var cfg Config
	err = v.Unmarshal(&cfg)
	if err != nil {
		log.Errorf("Unable to decode config into struct: %v", err)
		return nil, err
	}

	// --- Post-processing (e.g., convert seconds to duration) ---
	// Viper doesn't directly unmarshal into time.Duration from a number easily,
	// so we handle it manually if needed based on the field name/tag.
	// We used a distinct name `AGENT_CALL_TIMEOUT_SECONDS` which Viper unmarshals
	// as an int/float. Let's convert it.
	timeoutSeconds := v.GetInt("AGENT_CALL_TIMEOUT_SECONDS") // Use viper's GetInt
	cfg.AgentCallTimeout = time.Duration(timeoutSeconds) * time.Second

	loadedProviders := []string{}
    for name, providerCfg := range cfg.LLMProviders {
        if providerCfg.APIKey != "" {
             loadedProviders = append(loadedProviders, name)
        }
    }
    log.Infof("LLM Providers configured with API keys: %v", loadedProviders)
	
	// Validate log level
	_, err = log.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warnf("Invalid LOG_LEVEL '%s' found in config, defaulting to 'info'", cfg.LogLevel)
		cfg.LogLevel = "info"
	}

	log.Infof("Configuration loaded successfully (Port: %d, DB: %s..., LogLevel: %s)", cfg.HTTPPort, cfg.DatabaseURL[:min(15, len(cfg.DatabaseURL))], cfg.LogLevel)

	return &cfg, nil
}

// Helper function min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}