// internal/repository/repository.go
package repository

import (
	"context"

	"github.com/kathir-ks/a2a-platform/internal/models" // Internal models
	"github.com/kathir-ks/a2a-platform/pkg/a2a"         // For TaskStatus history
)

// TaskRepository defines the persistence operations for Tasks.
type TaskRepository interface {
	// Create saves a new task.
	Create(ctx context.Context, task *models.Task) (*models.Task, error)
	// Update modifies an existing task (typically status, artifacts, push config).
	Update(ctx context.Context, task *models.Task) (*models.Task, error)
	// FindByID retrieves a task by its ID. Should return a specific error
	// (e.g., sql.ErrNoRows or a custom ErrNotFound) if not found.
	FindByID(ctx context.Context, id string) (*models.Task, error)
	// FindBySessionID retrieves tasks associated with a session (optional).
	FindBySessionID(ctx context.Context, sessionID string) ([]*models.Task, error)
	// SetPushConfig saves or updates the push notification config for a task.
	SetPushConfig(ctx context.Context, taskID string, config *models.PushNotificationConfig) error
	// GetPushConfig retrieves the push notification config for a task.
	// Should return an error compatible with sql.ErrNoRows if not found.
	GetPushConfig(ctx context.Context, taskID string) (*models.PushNotificationConfig, error)
    // AddHistory appends a status entry to the task's history log.
    AddHistory(ctx context.Context, taskID string, status a2a.TaskStatus) error
    // GetHistory retrieves the last N status entries for a task.
    GetHistory(ctx context.Context, taskID string, limit int) ([]a2a.TaskStatus, error)
	// Delete (optional)
	// Delete(ctx context.Context, id string) error
}

// AgentRepository defines persistence operations for Agents.
type AgentRepository interface {
	Create(ctx context.Context, agent *models.Agent) (*models.Agent, error)
	Update(ctx context.Context, agent *models.Agent) (*models.Agent, error)
	FindByID(ctx context.Context, id string) (*models.Agent, error)
	FindByURL(ctx context.Context, url string) (*models.Agent, error)
	List(ctx context.Context, limit, offset int) ([]*models.Agent, error)
	// Delete(ctx context.Context, id string) error
    // SetEnabled(ctx context.Context, id string, isEnabled bool) error
}

// ToolRepository defines persistence operations for Tools.
type ToolRepository interface {
    Create(ctx context.Context, tool *models.ToolDefinition) (*models.ToolDefinition, error)
    FindByName(ctx context.Context, name string) (*models.ToolDefinition, error)
    List(ctx context.Context, limit, offset int) ([]*models.ToolDefinition, error)
    // Delete(ctx context.Context, name string) error
}

// Add other repository interfaces as needed (e.g., UserRepository)