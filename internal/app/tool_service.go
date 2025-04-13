// internal/app/tool_service.go
package app

import (
	"context"
	"fmt"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	"github.com/kathir-ks/a2a-platform/internal/tools"
)

type toolService struct {
	repo     repository.ToolRepository // Can be nil if not persisting definitions
	registry tools.Registry
}

// --- FIX: Add constructor ---
// NewToolService creates a new ToolService.
func NewToolService(deps ToolServiceDeps) ToolService { // Implements app.ToolService interface
	if deps.Registry == nil {
		panic("ToolService requires a non-nil Registry")
	}
	return &toolService{
		repo:     deps.ToolRepo, // Assign repo (can be nil)
		registry: deps.Registry,
	}
}

// Implement ToolService interface methods using s.registry (and s.repo if needed)
func (s *toolService) RegisterTool(ctx context.Context, tool models.ToolDefinition) error {
    // Example: Persist if repo exists
    // if s.repo != nil {
    //    _, err := s.repo.Create(ctx, &tool)
    //    if err != nil { return err }
    // }
    // Runtime registration would typically happen via toolRegistry.Register() in main.go
	return fmt.Errorf("dynamic tool registration via service not fully implemented")
}

func (s *toolService) GetToolSchema(ctx context.Context, toolName string) (any, error) {
	executor, err := s.registry.Get(ctx, toolName)
	if err != nil {
		return nil, err // Propagate registry error
	}
	return executor.GetDefinition().Schema, nil
}

func (s *toolService) ExecuteTool(ctx context.Context, toolName string, params map[string]any) (map[string]any, error) {
	executor, err := s.registry.Get(ctx, toolName)
	if err != nil {
		return nil, err // Propagate registry error
	}
	// Add potential authorization/validation logic here before executing
	return executor.Execute(ctx, params)
}

func (s *toolService) ListTools(ctx context.Context) ([]models.ToolDefinition, error) {
    // Primarily list from the runtime registry
	return s.registry.List(ctx)
    // Optionally, merge with or list from s.repo if definitions are also persisted
}