// internal/app/platform_service.go
package app

import (
	"context"
	"fmt"
    "time"

	"github.com/google/uuid" // For generating IDs if needed
	"github.com/kathir-ks/a2a-platform/internal/agentruntime"
	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

type platformService struct {
	taskSvc    TaskService
	agentRtCli agentruntime.Client
}

// NewPlatformService creates a new PlatformService implementation.
func NewPlatformService(deps PlatformServiceDeps) PlatformService {
	if deps.TaskSvc == nil || deps.AgentRtCli == nil {
		log.Fatal("PlatformService requires non-nil TaskService and AgentRuntimeClient")
	}
	return &platformService{
		taskSvc:    deps.TaskSvc,
		agentRtCli: deps.AgentRtCli,
	}
}

func (s *platformService) HandleSendTask(ctx context.Context, params a2a.TaskSendParams) (*a2a.Task, *a2a.JSONRPCError) {
	log.WithFields(log.Fields{"task_id": params.ID, "session_id": params.SessionID}).Info("Handling SendTask request in PlatformService")

	// 1. Determine Target Agent URL
	targetAgentURL, taskErr := s.taskSvc.GetTargetAgentURLForTask(ctx, params.ID)
	if taskErr != nil {
		// Could be TaskNotFound or InternalError from GetTargetAgentURLForTask
		log.Warnf("Failed to get target agent URL for task %s: %v", params.ID, taskErr)
		return nil, taskErr
	}
	log.Infof("Target agent URL for task %s: %s", params.ID, targetAgentURL)

	// 2. Prepare the request to forward
	// The incoming request encapsulates the necessary details.
	// We might generate a *new* request ID for the downstream call.
	forwardRequestID := uuid.NewString()
	forwardRequest := a2a.SendTaskRequest{
		JSONRPCMessage: a2a.JSONRPCMessage{
			JSONRPC: a2a.JSONRPCVersion,
			ID:      forwardRequestID, // Use a new ID for tracking the forwarded request
		},
		Method: a2a.MethodSendTask,
		Params: params, // Forward the original params
	}

	// 3. Update local task status (Optional: Mark as 'working' before sending)
    // This depends on desired semantics. Let's assume the external agent's response dictates the final state.
    // We *could* update status here, but need to handle potential failure of the downstream call.
    // For simplicity now, we'll update *after* getting the response.

	// 4. Send request to Target Agent via Agent Runtime Client
	log.Infof("Forwarding SendTask request (ID: %s) for task %s to %s", forwardRequestID, params.ID, targetAgentURL)
	responseBytes, agentErr := s.agentRtCli.SendRequest(ctx, targetAgentURL, &forwardRequest) // Pass pointer
	if agentErr != nil {
		log.Errorf("Error sending request to agent %s for task %s: %v", targetAgentURL, params.ID, agentErr)
		// This is an internal error from the platform's perspective (failed communication)
		// TODO: Should we update the task status to 'failed' here? Requires careful thought.
		return nil, a2a.NewInternalError(map[string]any{
			"details": "failed to communicate with target agent",
			"target": targetAgentURL,
			"cause": agentErr.Error(),
			})
	}

	// 5. Decode Response from Target Agent
	var agentResponse a2a.SendTaskResponse
	if err := json.Unmarshal(responseBytes, &agentResponse); err != nil {
		log.Errorf("Failed to decode response from agent %s for task %s: %v", targetAgentURL, params.ID, err)
        // TODO: Update task status to 'failed'?
		return nil, a2a.NewInternalError(map[string]any{
			"details": "invalid response received from target agent",
            "target": targetAgentURL,
			})
	}

	// Check if the agent returned an error in the JSON-RPC response
	if agentResponse.Error != nil {
		log.Warnf("Target agent %s returned error for task %s: [%d] %s", targetAgentURL, params.ID, agentResponse.Error.Code, agentResponse.Error.Message)
		// Update local task status to failed, potentially using agent's error details
        now := time.Now().UTC()
        failStatus := models.TaskStatus{
            State: models.TaskStateFailed,
            Timestamp: &now,
            Message: &models.Message{ // Assuming internal Message model
                Role: a2a.RoleAgent, // Platform reporting agent failure
                Parts: []any{ // Assuming internal Part representation
                    models.TextPart{Type: a2a.PartTypeText, Text: fmt.Sprintf("Agent error: [%d] %s", agentResponse.Error.Code, agentResponse.Error.Message)},
                },
            },
        }
		taskModel, getErr := s.taskSvc.GetTaskByID(ctx, params.ID)
		if getErr == nil {
            taskModel.Status = failStatus
            _, updateErr := s.taskSvc.UpdateTask(ctx, taskModel) // Update internal task model
            if updateErr != nil {
                 log.Errorf("Failed to update task %s to failed status after agent error: %v", params.ID, updateErr)
            }
        } else {
            log.Errorf("Failed to get task %s to update status after agent error: %v", params.ID, getErr)
        }
        // Propagate the error from the agent
		return nil, agentResponse.Error
	}

    // 6. Agent responded successfully with a Task object
    if agentResponse.Result == nil {
        log.Errorf("Target agent %s returned success but nil result for task %s", targetAgentURL, params.ID)
        // TODO: Update task status to 'failed'? This is unexpected success state.
		return nil, a2a.NewInternalError(map[string]any{
			"details": "agent returned success response with no result task data",
            "target": targetAgentURL,
			})
    }

	log.Infof("Received successful task update from agent %s for task %s. New status: %s", targetAgentURL, params.ID, agentResponse.Result.Status.State)
	// Convert the received A2A Task result to our internal model format
	updatedTaskModel := models.TaskA2AToModel(agentResponse.Result) // Need conversion

	// 7. Update Local Task State
	// Use UpdateTask which also handles history
	finalTaskModel, updateErr := s.taskSvc.UpdateTask(ctx, updatedTaskModel)
	if updateErr != nil {
		log.Errorf("Failed to update local task %s after successful agent response: %v", params.ID, updateErr)
		// Return the agent's successful result, but log the persistence error
		// This is tricky - the operation succeeded externally but failed internally.
		// Return the agent result for now, as the task *did* progress.
		return agentResponse.Result, nil // Or return an internal error? Discuss semantics.
	}

	// 8. Return the final task state (converted back to A2A type)
	return models.TaskModelToA2A(finalTaskModel), nil // Use the state confirmed by the database update
}


// HandleSendTaskSubscribe - Placeholder - Complex implementation needed
func (s *platformService) HandleSendTaskSubscribe(ctx context.Context, params a2a.TaskSendParams) (<-chan any, *a2a.JSONRPCError) {
	log.Warn("HandleSendTaskSubscribe not implemented")
    // 1. Check if target agent supports streaming (via AgentService/Capabilities)
    // 2. Call agent runtime client's streaming method
    // 3. Set up channels to pipe events back
    // 4. Coordinate with WebSocket manager (ws.Manager) to send events to the correct client
	return nil, a2a.NewUnsupportedOperationError("streaming send")
}

// HandleResubscribeTask - Placeholder - Complex implementation needed
func (s *platformService) HandleResubscribeTask(ctx context.Context, params a2a.TaskQueryParams) (<-chan any, *a2a.JSONRPCError) {
	log.Warn("HandleResubscribeTask not implemented")
    // 1. Find existing task stream/state
    // 2. Re-establish connection/subscription if necessary (might involve agent runtime client)
    // 3. Coordinate with WebSocket manager
	return nil, a2a.NewUnsupportedOperationError("streaming resubscribe")
}


// --- Add AgentService and ToolService implementations similarly ---
// internal/app/agent_service.go
// internal/app/tool_service.go
// These would depend on their respective repositories (repository.AgentRepository, etc.)