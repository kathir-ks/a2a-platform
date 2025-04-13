// internal/app/agent_service.go
package app

import (
	"context"
	"github.com/google/uuid" // For generating Agent IDs
	"time"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
    log "github.com/sirupsen/logrus"
)

type agentService struct {
	repo repository.AgentRepository
}

// NewAgentService creates a new AgentService.
func NewAgentService(deps AgentServiceDeps) AgentService {
	if deps.AgentRepo == nil {
		panic("AgentService requires a non-nil AgentRepository")
	}
	return &agentService{repo: deps.AgentRepo}
}

// Implement AgentService methods using r.repo...
func (s *agentService) RegisterAgent(ctx context.Context, card a2a.AgentCard) (*models.Agent, error) {
    // Basic implementation - generate ID, create model, save
     agentID := uuid.NewString() // Generate a unique ID
     now := time.Now().UTC()
     agentModel := &models.Agent{
         ID: agentID,
         AgentCard: models.AgentCard(card), // Convert/cast if needed
         RegisteredAt: now,
         LastUpdatedAt: now,
         IsEnabled: true, // Default to enabled
     }
     created, err := s.repo.Create(ctx, agentModel)
     if err != nil {
         log.Errorf("Failed to register agent '%s': %v", card.Name, err)
         return nil, err // Or return a structured error
     }
     log.Infof("Registered agent '%s' with ID %s", card.Name, agentID)
     return created, nil
}

func (s *agentService) GetAgent(ctx context.Context, agentID string) (*models.Agent, error) {
	return s.repo.FindByID(ctx, agentID) // Add error mapping if needed
}
 func (s *agentService) GetAgentByURL(ctx context.Context, url string) (*models.Agent, error) {
    return s.repo.FindByURL(ctx, url) // Add error mapping if needed
}
// ... implement other methods ...