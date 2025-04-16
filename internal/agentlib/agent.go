// internal/agentlib/agent.go
package agentlib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

// Agent represents a runnable A2A agent service.
type Agent struct {
	config      *Config
	card        a2a.AgentCard
	taskHandler TaskHandler // The implementation handling task logic
	httpServer  *http.Server
	serverLogic *agentServer // Holds the HTTP routing logic
}

// NewAgent creates a new agent instance.
// It requires agent configuration and a TaskHandler implementation.
func NewAgent(cfg *Config, handler TaskHandler) (*Agent, error) {
	if cfg == nil {
		return nil, errors.New("agent configuration cannot be nil")
	}
	if handler == nil {
		return nil, errors.New("agent task handler cannot be nil")
	}
	if cfg.AgentURL == "" {
		log.Warnf("Agent URL is not set in config, AgentCard URL will be empty.")
	}

	card := a2a.AgentCard{
		Name:           cfg.Name,
		Description:    &cfg.Description, // Use address for optional string
		URL:            cfg.AgentURL,     // Crucial: Needs to be reachable by the platform
		Version:        cfg.Version,
		Capabilities:   cfg.Capabilities,
		Authentication: cfg.Authentication,
		Skills:         cfg.Skills,
		// Provider, DocsURL, Modes can be added if needed
	}
	// Basic validation
	if card.Name == "" || card.Version == "" || card.URL == "" {
		return nil, fmt.Errorf("agent card requires Name, Version, and URL (found Name='%s', Version='%s', URL='%s')", card.Name, card.Version, card.URL)
	}


	agent := &Agent{
		config:      cfg,
		card:        card,
		taskHandler: handler,
	}
	agent.serverLogic = &agentServer{agent: agent} // Link server logic back to agent

	return agent, nil
}

// GetCard returns the AgentCard describing this agent.
func (a *Agent) GetCard() a2a.AgentCard {
	return a.card
}

// Start runs the agent's HTTP server.
// It blocks until the server is shut down. Call Stop() for graceful shutdown.
func (a *Agent) Start() error {
	if a.httpServer != nil {
		return errors.New("agent server already started")
	}

	router := a.serverLogic.NewRouter() // Get the configured router

	listenAddr := fmt.Sprintf(":%d", a.config.Port)
	a.httpServer = &http.Server{
		Addr:         listenAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Infof("Starting Agent '%s' v%s on %s%s", a.config.Name, a.config.Version, listenAddr, a.config.EndpointPath)
	log.Infof("Agent Card URL: %s", a.card.URL)

	// Start listening
	if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Errorf("Agent '%s' HTTP server error: %v", a.config.Name, err)
		a.httpServer = nil // Ensure it's nil if ListenAndServe fails immediately
		return err
	}

	log.Infof("Agent '%s' server stopped.", a.config.Name)
	return nil
}

// Stop gracefully shuts down the agent's HTTP server.
func (a *Agent) Stop(ctx context.Context) error {
	if a.httpServer == nil {
		log.Warnf("Agent '%s' server is not running, cannot stop.", a.config.Name)
		return nil // Not an error if already stopped
	}

	log.Infof("Shutting down Agent '%s' server...", a.config.Name)
	err := a.httpServer.Shutdown(ctx)
	if err != nil {
		log.Errorf("Agent '%s' graceful shutdown failed: %v", a.config.Name, err)
		// Fallback to close if shutdown fails after context deadline
		if closeErr := a.httpServer.Close(); closeErr != nil {
			log.Errorf("Agent '%s' server close failed: %v", a.config.Name, closeErr)
		}
		return err // Return the shutdown error
	}

	a.httpServer = nil // Mark as stopped
	log.Infof("Agent '%s' server shutdown complete.", a.config.Name)
	return nil
}