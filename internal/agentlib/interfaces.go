// internal/agentlib/interfaces.go
package agentlib

import (
	"context"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
)

// TaskHandler defines the interface that agent implementations must satisfy
// to process incoming task requests.
type TaskHandler interface {
	// HandleTaskSend processes an incoming message for a task.
	// It should perform the agent's logic based on the params.Message
	// and return the resulting task state (often with updated Status or Artifacts)
	// or an A2A JSON-RPC error if processing fails.
	HandleTaskSend(ctx context.Context, params a2a.TaskSendParams) (*a2a.Task, *a2a.JSONRPCError)

	// HandleGetTask (Optional) processes a request to get task status.
	// Many simple agents might not store state and can return UnsupportedOperation.
	// HandleGetTask(ctx context.Context, params a2a.TaskQueryParams) (*a2a.Task, *a2a.JSONRPCError)

	// HandleCancelTask (Optional) processes a request to cancel a task.
	// Returns the task state after attempting cancellation.
	// HandleCancelTask(ctx context.Context, params a2a.TaskIdParams) (*a2a.Task, *a2a.JSONRPCError)
}