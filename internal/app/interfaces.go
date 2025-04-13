// internal/app/interfaces.go
package app

import (
	"context"

	"github.com/kathir-ks/a2a-platform/internal/models" // Internal data models
	"github.com/kathir-ks/a2a-platform/pkg/a2a"         // A2A schema types
	"github.com/kathir-ks/a2a-platform/internal/agentruntime" // Agent client
	"github.com/kathir-ks/a2a-platform/internal/repository" // Repository interfaces
)

// PlatformService orchestrates high-level agent interactions.
// It's the primary entry point for complex requests like tasks/send
// that might involve calling other agents.
type PlatformService interface {
	// HandleSendTask orchestrates sending a message to a task, which might involve
	// forwarding the request to an external agent.
	HandleSendTask(ctx context.Context, params a2a.TaskSendParams) (*a2a.Task, *a2a.JSONRPCError)

	// HandleSendTaskSubscribe orchestrates sending a message and establishing
	// a streaming subscription. It will likely involve calling the agent runtime client
	// in a way that supports streaming (if the target agent supports it).
	// The return channel will emit events (status/artifact updates) or errors.
	// THIS IS COMPLEX - simplified initial return signature. Actual implementation
	// will need more robust channel management tied to the WS connection manager.
	HandleSendTaskSubscribe(ctx context.Context, params a2a.TaskSendParams) (<-chan any, *a2a.JSONRPCError) // Returns channel for events/errors

    // HandleResubscribeTask re-establishes a stream for an existing task.
    HandleResubscribeTask(ctx context.Context, params a2a.TaskQueryParams) (<-chan any, *a2a.JSONRPCError) // Returns channel for events/errors
}

// TaskService manages the lifecycle and state of tasks within the platform.
type TaskService interface {
	// HandleGetTask retrieves task details based on query parameters.
	HandleGetTask(ctx context.Context, params a2a.TaskQueryParams) (*a2a.Task, *a2a.JSONRPCError)
	// HandleCancelTask attempts to cancel a running task.
	HandleCancelTask(ctx context.Context, params a2a.TaskIdParams) (*a2a.Task, *a2a.JSONRPCError)
	// HandleSetTaskPushNotification sets or updates push notification settings for a task.
	HandleSetTaskPushNotification(ctx context.Context, params a2a.TaskPushNotificationConfig) (*a2a.TaskPushNotificationConfig, *a2a.JSONRPCError)
	// HandleGetTaskPushNotification retrieves push notification settings for a task.
	HandleGetTaskPushNotification(ctx context.Context, params a2a.TaskIdParams) (*a2a.TaskPushNotificationConfig, *a2a.JSONRPCError)

	// --- Internal methods (potentially called by PlatformService or other internal components) ---

	// GetTaskByID retrieves the internal representation of a task.
	GetTaskByID(ctx context.Context, taskID string) (*models.Task, *a2a.JSONRPCError)
	// CreateTask creates a new task record.
	CreateTask(ctx context.Context, taskData *models.Task) (*models.Task, *a2a.JSONRPCError) // Takes internal model
	// UpdateTask updates an existing task record (e.g., status, artifacts).
	UpdateTask(ctx context.Context, taskData *models.Task) (*models.Task, *a2a.JSONRPCError) // Takes internal model
    // AddTaskHistory records a status transition (if history is enabled).
    AddTaskHistory(ctx context.Context, taskID string, status a2a.TaskStatus) error
    // GetTaskHistory retrieves recent status history.
    GetTaskHistory(ctx context.Context, taskID string, limit int) ([]a2a.TaskStatus, error)
	// GetTargetAgentURLForTask retrieves the URL of the agent responsible for handling this task.
	// This is crucial for PlatformService to know where to forward requests.
	GetTargetAgentURLForTask(ctx context.Context, taskID string) (string, *a2a.JSONRPCError)
}

// AgentService manages agent definitions registered with the platform.
type AgentService interface {
	RegisterAgent(ctx context.Context, card a2a.AgentCard) (*models.Agent, error)
	GetAgent(ctx context.Context, agentID string) (*models.Agent, error)
	GetAgentByURL(ctx context.Context, url string) (*models.Agent, error)
	// ... other agent management methods (ListAgents, UpdateAgent, DeleteAgent)
}

// ToolService manages tools available on the platform.
type ToolService interface {
	RegisterTool(ctx context.Context, tool models.ToolDefinition) error
	GetToolSchema(ctx context.Context, toolName string) (any, error)
	ExecuteTool(ctx context.Context, toolName string, params map[string]any) (map[string]any, error)
	ListTools(ctx context.Context) ([]models.ToolDefinition, error)
}

// --- Service Dependencies ---

// Services might depend on repositories and other clients/services.
type TaskServiceDeps struct {
	TaskRepo repository.TaskRepository
    // Potentially add a reference to a notification service here later
}

type PlatformServiceDeps struct {
	TaskSvc    TaskService // Depends on TaskService
	AgentRtCli agentruntime.Client // Depends on the client to call external agents
    // May need AgentService later if more agent info is required during routing
}

// Add AgentServiceDeps, ToolServiceDeps as needed