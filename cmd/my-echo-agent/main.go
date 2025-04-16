// cmd/my-echo-agent/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kathir-ks/a2a-platform/internal/agentlib" // Import the agent library
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

// EchoTaskHandler implements the agentlib.TaskHandler interface.
type EchoTaskHandler struct{}

// HandleTaskSend simply echoes the incoming message parts back in the result.
func (h *EchoTaskHandler) HandleTaskSend(ctx context.Context, params a2a.TaskSendParams) (*a2a.Task, *a2a.JSONRPCError) {
	log.Infof("EchoAgent processing task %s", params.ID)

	// Create a response message mirroring the input
	responseMessage := a2a.Message{
		Role:  a2a.RoleAgent, // Respond as the agent
		Parts: params.Message.Parts, // Echo the parts directly
	}

	// Simulate work and completion
	now := time.Now().UTC()
	completedStatus := a2a.TaskStatus{
		State:     a2a.TaskStateCompleted,
		Message:   &responseMessage,
		Timestamp: &now,
	}

	// Construct the final task state to return
	// Important: The agent typically *doesn't* manage the full task history or artifacts
	// unless it's a complex stateful agent. It just returns the *current* state result.
	resultTask := &a2a.Task{
		ID:        params.ID,
		SessionID: params.SessionID,
		Status:    completedStatus,
		// Artifacts could be added here if the agent generated any
	}

	log.Infof("EchoAgent completed task %s", params.ID)
	return resultTask, nil // Return the completed task state, no error
}

func main() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetLevel(log.DebugLevel) // Be verbose for example

	// --- Agent Configuration ---
	// Load from environment or use defaults
	cfg := agentlib.LoadConfigFromEnv()
	// Override specific settings if needed for this agent
	cfg.Name = "EchoAgent"
	cfg.Description = "A simple agent that echoes back the input message text."
	cfg.Port = 9091 // Use a different port than the platform
	cfg.EndpointPath = "/echo/a2a"
	cfg.AgentURL = fmt.Sprintf("http://localhost:%d%s", cfg.Port, cfg.EndpointPath) // Ensure this is correct!
	cfg.Skills = []a2a.AgentSkill{{ID: "echo", Name: "Echo Input", Description: &cfg.Description}}

	// --- Create Handler and Agent ---
	handler := &EchoTaskHandler{}
	agent, err := agentlib.NewAgent(cfg, handler)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// --- Start Agent Server ---
	go func() {
		if err := agent.Start(); err != nil {
			log.Errorf("Agent server failed: %v", err)
			// Maybe trigger shutdown or handle error
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down EchoAgent...")

	// Allow time for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := agent.Stop(ctx); err != nil {
		log.Fatalf("Agent shutdown failed: %v", err)
	}

	log.Info("EchoAgent stopped.")
}