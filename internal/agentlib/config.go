// internal/agentlib/config.go
package agentlib

import (
	"os"
	"strconv"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
)

// Config holds the configuration for a standalone agent.
type Config struct {
	Name           string // Agent's display name
	Version        string // Agent's version
	Description    string // Agent's description
	Port           int    // Port the agent listens on
	EndpointPath   string // Path for the A2A endpoint (e.g., "/a2a")
	AgentURL       string // Full public URL (e.g., "http://my-agent.com:9090/a2a") - needed for AgentCard
	Skills         []a2a.AgentSkill
	Capabilities   a2a.AgentCapabilities
	Authentication *a2a.AgentAuthentication // Optional
	// Add other agent-specific config if needed
}

// DefaultConfig creates a basic default configuration.
func DefaultConfig() *Config {
	return &Config{
		Name:         "MyA2AAgent",
		Version:      "0.1.0",
		Description:  "A basic A2A agent",
		Port:         9090,
		EndpointPath: "/a2a",
		AgentURL:     "http://localhost:9090/a2a", // Needs to be accurate
		Skills: []a2a.AgentSkill{
			{ID: "default", Name: "Default Skill"},
		},
		Capabilities: a2a.AgentCapabilities{ // Default capabilities
			Streaming:             false,
			PushNotifications:     false,
			StateTransitionHistory: false,
		},
	}
}

// LoadConfigFromEnv loads configuration, overriding defaults with environment variables.
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if name := os.Getenv("AGENT_NAME"); name != "" {
		cfg.Name = name
	}
	if version := os.Getenv("AGENT_VERSION"); version != "" {
		cfg.Version = version
	}
	if desc := os.Getenv("AGENT_DESCRIPTION"); desc != "" {
		cfg.Description = desc
	}
	if portStr := os.Getenv("AGENT_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = port
		}
	}
	if path := os.Getenv("AGENT_ENDPOINT_PATH"); path != "" {
		cfg.EndpointPath = path
	}
	if url := os.Getenv("AGENT_URL"); url != "" {
		cfg.AgentURL = url
	}
	// TODO: Load Skills, Capabilities, Auth from env (e.g., JSON string) if needed

	return cfg
}