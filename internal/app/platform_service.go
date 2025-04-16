// internal/app/platform_service.go
package app

import (
	"context"
	"fmt"
    "time"
	"encoding/json"
	"strings"

	"github.com/google/uuid" // For generating IDs if needed
	"github.com/kathir-ks/a2a-platform/internal/agentruntime"
	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

type platformService struct {
	taskSvc    TaskService
	agentRtCli agentruntime.Client
	agentSvc   AgentService
}

// NewPlatformService creates a new PlatformService implementation.
func NewPlatformService(deps PlatformServiceDeps) PlatformService {
	if deps.TaskSvc == nil || deps.AgentRtCli == nil || deps.AgentSvc == nil {
		log.Fatal("PlatformService requires non-nil TaskService and AgentRuntimeClient")
	}
	return &platformService{
		taskSvc:    deps.TaskSvc,
		agentRtCli: deps.AgentRtCli,
		agentSvc:   deps.AgentSvc,
	}
}
func (s *platformService) HandleSendTask(ctx context.Context, params a2a.TaskSendParams) (*a2a.Task, *a2a.JSONRPCError) {
	log.WithFields(log.Fields{"task_id": params.ID, "session_id": params.SessionID}).Info("Handling SendTask request in PlatformService")

	var targetAgentURL string
	var targetAgentID string // Store the agent ID too

	// 1. Try to get existing task to find the agent URL
	taskModel, getErr := s.taskSvc.GetTaskByID(ctx, params.ID)

	if getErr != nil {
		// Check if the error is specifically "Task Not Found"
		if getErr.Code == a2a.CodeTaskNotFound { // Check the JSON-RPC error code from TaskService
			log.Infof("Task %s not found, treating as new task.", params.ID)

			// --- Logic for NEW task ---
			// How do we know which agent to target? This is the tricky part.
			// For this example, let's assume the task ID or metadata tells us,
			// or we hardcode it for the "echo-task-*" prefix.
			// **REAL IMPLEMENTATION NEEDS A BETTER ROUTING MECHANISM**
			var agentName string
			if strings.HasPrefix(params.ID, "echo-task-") {
				agentName = "EchoAgent" // Hardcoded lookup for demo
			} else if strings.HasPrefix(params.ID, "calc-task-") {
				agentName = "CalculatorAgent" // Example
			} else {
				log.Errorf("Cannot determine target agent for new task ID: %s", params.ID)
				return nil, a2a.NewInvalidRequestError(map[string]string{
					"details": "Cannot determine target agent for new task ID",
					"taskId":  params.ID,
				})
			}

			// Find the agent by name using AgentService
			// Note: FindByName is not in the current AgentRepository interface, add it or use List/FindByURL
			// Let's assume GetAgentByURL exists and we use the registered URL
			// We really need a FindByName or similar. Adding a temporary FindByURL lookup:
			log.Warnf("Attempting agent lookup by hardcoded name '%s'. Finding by name/metadata is preferred.", agentName)
			// This lookup is inefficient, needs improvement (e.g., add FindByName to AgentService/Repo)
			registeredAgents, listErr := s.agentSvc.ListAgents(ctx, 100, 0) // Assuming ListAgents exists in AgentService
			var foundAgent *models.Agent = nil
			if listErr == nil {
				for _, agent := range registeredAgents {
					if agent.AgentCard.Name == agentName {
						foundAgent = agent
						break
					}
				}
			}

			if foundAgent == nil {
				log.Errorf("Target agent '%s' for new task %s not found in registry.", agentName, params.ID)
				return nil, a2a.NewInternalError(map[string]string{
					"details": "Target agent definition not found",
					"agent":   agentName,
				})
			}

			targetAgentURL = foundAgent.AgentCard.URL
			targetAgentID = foundAgent.ID // Store the agent's platform ID
			log.Infof("Found agent '%s' (ID: %s) at URL %s for new task %s", agentName, targetAgentID, targetAgentURL, params.ID)

			// Create the task record in the repository
			now := time.Now().UTC()
			initialStatus := models.TaskStatus{
				State:     models.TaskStateSubmitted, // Start as submitted
				Timestamp: &now,
				// Message: nil initially
			}
			newTask := &models.Task{
				ID:             params.ID,
				SessionID:      params.SessionID,
				Status:         initialStatus,
				TargetAgentURL: targetAgentURL,
				TargetAgentID:  targetAgentID,
				CreatedAt:      now,
				LastUpdatedAt:  now,
				Metadata:       params.Metadata,
			}
			_, createErr := s.taskSvc.CreateTask(ctx, newTask)
			if createErr != nil {
				log.Errorf("Failed to create new task record for %s: %v", params.ID, createErr)
				return nil, createErr // Return the error from CreateTask
			}
			log.Infof("Created new task record for %s", params.ID)

		} else {
			// A different error occurred trying to fetch the task
			log.Errorf("Error fetching task %s for routing: %v (Code: %d)", params.ID, getErr, getErr.Code)
			// Return the specific error you saw
			return nil, a2a.NewInternalError(map[string]string{
				"details": "failed to retrieve task data for routing",
				"cause":   getErr.Error(),
			})
		}
	} else {
		// --- Logic for EXISTING task ---
		log.Infof("Found existing task %s", params.ID)
		targetAgentURL = taskModel.TargetAgentURL
		targetAgentID = taskModel.TargetAgentID // Get existing agent ID
		if targetAgentURL == "" {
			log.Errorf("Existing task %s has no target agent URL defined!", params.ID)
			return nil, a2a.NewInternalError(map[string]string{
				"details": "task configuration error: missing target agent URL",
				"taskId":  params.ID,
			})
		}
		log.Infof("Target agent URL for existing task %s: %s", params.ID, targetAgentURL)
	}


	// --- CONTINUE with sending request to agent ---

	// 2. Prepare the request to forward
	forwardRequestID := uuid.NewString()
	forwardRequest := a2a.SendTaskRequest{
		JSONRPCMessage: a2a.JSONRPCMessage{
			JSONRPC: a2a.JSONRPCVersion,
			ID:      forwardRequestID,
		},
		Method: a2a.MethodSendTask,
		Params: params,
	}

	// 3. Send request to Target Agent via Agent Runtime Client
	log.Infof("Forwarding SendTask request (ID: %s) for task %s to %s", forwardRequestID, params.ID, targetAgentURL)
	genericForwardRequest := &a2a.JSONRPCRequest{
		JSONRPCMessage: forwardRequest.JSONRPCMessage,
		Method:         forwardRequest.Method,
		Params:         forwardRequest.Params,
	}

	responseBytes, agentErr := s.agentRtCli.SendRequest(ctx, targetAgentURL, genericForwardRequest)
	if agentErr != nil {
		log.Errorf("Error sending request to agent %s for task %s: %v", targetAgentURL, params.ID, agentErr)
		// Update task status to failed?
		// now := time.Now().UTC()
		failMsg := fmt.Sprintf("Platform failed to reach agent: %v", agentErr)
		s.tryUpdateTaskStatus(ctx, params.ID, models.TaskStateFailed, failMsg)
		return nil, a2a.NewInternalError(map[string]any{
			"details": "failed to communicate with target agent",
			"target":  targetAgentURL,
			"cause":   agentErr.Error(),
		})
	}

	// 4. Decode Response from Target Agent
	var agentResponse a2a.SendTaskResponse
	if err := json.Unmarshal(responseBytes, &agentResponse); err != nil {
		log.Errorf("Failed to decode response from agent %s for task %s: %v", targetAgentURL, params.ID, err)
		// Update task status to failed?
		failMsg := fmt.Sprintf("Platform failed to decode agent response: %v", err)
		s.tryUpdateTaskStatus(ctx, params.ID, models.TaskStateFailed, failMsg)
		return nil, a2a.NewInternalError(map[string]any{
			"details": "invalid response received from target agent",
			"target":  targetAgentURL,
		})
	}

	// 5. Check for JSON-RPC error from agent
	if agentResponse.Error != nil {
		log.Warnf("Target agent %s returned error for task %s: [%d] %s", targetAgentURL, params.ID, agentResponse.Error.Code, agentResponse.Error.Message)
		// Update local task status to failed, using agent's error details
		failMsg := fmt.Sprintf("Agent error: [%d] %s", agentResponse.Error.Code, agentResponse.Error.Message)
		s.tryUpdateTaskStatus(ctx, params.ID, models.TaskStateFailed, failMsg)
		// Propagate the error from the agent
		return nil, agentResponse.Error
	}

	// 6. Agent responded successfully with a Task object
	if agentResponse.Result == nil {
		log.Errorf("Target agent %s returned success but nil result for task %s", targetAgentURL, params.ID)
		// Update task status to failed? This is unexpected success state.
		failMsg := "Agent returned success response with no result task data"
		s.tryUpdateTaskStatus(ctx, params.ID, models.TaskStateFailed, failMsg)
		return nil, a2a.NewInternalError(map[string]any{
			"details": failMsg,
			"target":  targetAgentURL,
		})
	}

	log.Infof("Received successful task update from agent %s for task %s. New status: %s", targetAgentURL, params.ID, agentResponse.Result.Status.State)

	// Convert the received A2A Task result to our internal model format
	updatedTaskModel, convErr := models.TaskA2AToModel(agentResponse.Result)
	if convErr != nil {
		log.Errorf("Failed to convert agent response task %s to internal model: %v", agentResponse.Result.ID, convErr)
		// Returning internal error is safer. Don't update local state if we can't parse agent response.
		return nil, a2a.NewInternalError(map[string]string{"details": "failed processing agent response"})
	}
	if updatedTaskModel == nil {
		log.Errorf("Internal consistency error: TaskA2AToModel returned nil model without error for task %s", agentResponse.Result.ID)
		return nil, a2a.NewInternalError(map[string]string{"details": "internal error processing agent response"})
	}

	// Ensure the TargetAgentURL/ID are preserved from the existing/newly created record
	// The agent's response shouldn't overwrite these routing details.
	updatedTaskModel.TargetAgentURL = targetAgentURL
	updatedTaskModel.TargetAgentID = targetAgentID
	// Also preserve CreatedAt from the original model if it existed
	if taskModel != nil { // taskModel is the one we fetched/created earlier
        updatedTaskModel.CreatedAt = taskModel.CreatedAt
    }


	// 7. Update Local Task State
	finalTaskModel, updateErr := s.taskSvc.UpdateTask(ctx, updatedTaskModel)
	if updateErr != nil {
		log.Errorf("Failed to update local task %s after successful agent response: %v", params.ID, updateErr)
		// Return the agent's successful result, but log the persistence error.
		// The task *did* progress externally.
		return agentResponse.Result, nil // Or return an internal error? Let's return agent result for now.
	}

	// 8. Return the final task state (converted back to A2A type)
	taskA2A, convErr := models.TaskModelToA2A(finalTaskModel)
	if convErr != nil {
		log.Errorf("Failed to convert final task %s to A2A model for response: %v", finalTaskModel.ID, convErr)
		return nil, a2a.NewInternalError(map[string]string{"details": "internal error finalizing response"})
	}
	return &taskA2A, nil // Return pointer to the converted value, and nil JSONRPC error
}

// Helper function to attempt updating task status, logs errors but doesn't return them
func (s *platformService) tryUpdateTaskStatus(ctx context.Context, taskID string, state models.TaskState, message string) {
	taskModel, getErr := s.taskSvc.GetTaskByID(ctx, taskID)
	if getErr != nil {
		log.Warnf("Failed to get task %s to update status to %s after error: %v", taskID, state, getErr)
		return
	}

	now := time.Now().UTC()
	taskModel.Status = models.TaskStatus{
		State: state,
		Timestamp: &now,
		Message: &models.Message{
			Role: a2a.RoleAgent, // Platform reporting the failure
			Parts: []models.Part{models.TextPart{Type: a2a.PartTypeText, Text: message}},
		},
	}

	_, updateErr := s.taskSvc.UpdateTask(ctx, taskModel)
	if updateErr != nil {
		log.Errorf("Failed to update task %s status to %s after error: %v", taskID, state, updateErr)
	} else {
        log.Infof("Updated task %s status to %s due to processing error.", taskID, state)
    }
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