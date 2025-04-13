// internal/app/task_service.go
package app

import (
	"context"
	"database/sql" // Needed for checking specific errors like sql.ErrNoRows
	"errors"       // Standard error handling
	"time"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

type taskService struct {
	repo repository.TaskRepository
	// Add other dependencies like a notification service later
}

// NewTaskService creates a new TaskService implementation.
func NewTaskService(deps TaskServiceDeps) TaskService {
	if deps.TaskRepo == nil {
		log.Fatal("TaskService requires a non-nil TaskRepository") // Or handle differently
	}
	return &taskService{
		repo: deps.TaskRepo,
	}
}

// --- Public API facing methods ---

func (s *taskService) HandleGetTask(ctx context.Context, params a2a.TaskQueryParams) (*a2a.Task, *a2a.JSONRPCError) {
	log.WithFields(log.Fields{"task_id": params.ID, "history_len": params.HistoryLength}).Info("Handling GetTask request")

	taskModel, err := s.repo.FindByID(ctx, params.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Warnf("Task not found for GetTask: %s", params.ID)
			return nil, a2a.NewTaskNotFoundError(params.ID)
		}
		log.Errorf("Error finding task %s: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to retrieve task"})
	}

	// Convert internal model to A2A type
	taskA2A := models.TaskModelToA2A(taskModel) // Need this conversion function

    // Fetch history if requested
    if params.HistoryLength != nil && *params.HistoryLength > 0 {
       // TODO: Implement history retrieval logic using GetTaskHistory
       log.Warn("Task history retrieval not fully implemented yet")
       // history, historyErr := s.GetTaskHistory(ctx, params.ID, *params.HistoryLength)
       // if historyErr != nil { ... handle error ... }
       // taskA2A.StatusHistory = history // Assuming Task A2A struct has a place for history
    }


	return taskA2A, nil
}

func (s *taskService) HandleCancelTask(ctx context.Context, params a2a.TaskIdParams) (*a2a.Task, *a2a.JSONRPCError) {
	log.WithField("task_id", params.ID).Info("Handling CancelTask request")

	taskModel, err := s.repo.FindByID(ctx, params.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, a2a.NewTaskNotFoundError(params.ID)
		}
		log.Errorf("Error finding task %s for cancellation: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to retrieve task for cancellation"})
	}

	// Check if task is in a cancelable state
	currentState := taskModel.Status.State // Assuming Status is stored in the model
	if !(currentState == models.TaskStateSubmitted || currentState == models.TaskStateWorking || currentState == models.TaskStateInputRequired) {
		log.Warnf("Task %s is not in a cancelable state (%s)", params.ID, currentState)
		// Return current task state but with error indicating non-cancelable
		return models.TaskModelToA2A(taskModel), a2a.NewTaskNotCancelableError(params.ID)
	}

	// Update status to Canceled
	now := time.Now().UTC()
	taskModel.Status = models.TaskStatus{ // Assuming internal TaskStatus model exists
        State:     models.TaskStateCanceled,
        Timestamp: &now,
        // Message: Optionally add a cancellation message
    }
    // Add history record
    if historyErr := s.AddTaskHistory(ctx, taskModel.ID, models.TaskStatusModelToA2A(taskModel.Status)); historyErr != nil {
        log.Errorf("Failed to add cancellation history for task %s: %v", taskModel.ID, historyErr)
        // Continue cancellation, but log the error
    }

	updatedTask, err := s.repo.Update(ctx, taskModel)
	if err != nil {
		log.Errorf("Error updating task %s to canceled state: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to update task status"})
	}

	return models.TaskModelToA2A(updatedTask), nil
}

func (s *taskService) HandleSetTaskPushNotification(ctx context.Context, params a2a.TaskPushNotificationConfig) (*a2a.TaskPushNotificationConfig, *a2a.JSONRPCError) {
	log.WithField("task_id", params.ID).Info("Handling SetTaskPushNotification request")

	// Fetch task to ensure it exists (optional, repo might handle upsert)
	_, err := s.repo.FindByID(ctx, params.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, a2a.NewTaskNotFoundError(params.ID)
		}
		log.Errorf("Error finding task %s for setting push notification: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to retrieve task"})
	}

	// Convert A2A config to internal model representation if necessary
	configModel := models.PushNotificationConfigA2AToModel(¶ms.PushNotificationConfig) // Need conversion

	if err := s.repo.SetPushConfig(ctx, params.ID, configModel); err != nil {
		log.Errorf("Error setting push config for task %s: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to save push notification config"})
	}

	// Return the config that was set (passed in params)
	return ¶ms, nil
}

func (s *taskService) HandleGetTaskPushNotification(ctx context.Context, params a2a.TaskIdParams) (*a2a.TaskPushNotificationConfig, *a2a.JSONRPCError) {
	log.WithField("task_id", params.ID).Info("Handling GetTaskPushNotification request")

	configModel, err := s.repo.GetPushConfig(ctx, params.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// It's debatable whether this is a TaskNotFound or just "no config set".
			// Let's return TaskNotFound for consistency, implying the task itself is the context.
			log.Warnf("Push config or task not found for GetTaskPushNotification: %s", params.ID)
			return nil, a2a.NewTaskNotFoundError(params.ID) // Or a different error?
		}
		log.Errorf("Error getting push config for task %s: %v", params.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to retrieve push notification config"})
	}

	// Check if a config was actually found (repo might return nil model, nil error if allowed)
	if configModel == nil {
        log.Warnf("No push config found for task %s, returning empty", params.ID)
		// Return null result according to JSON-RPC spec for 'not found' scenarios where the main entity (task) exists
		// Returning TaskNotFound might be misleading if the task *does* exist but has no config.
        // Let's return an empty successful response (result: null)
        // Alternatively, define a specific error like "PushConfigNotFound".
        // For now, let's align with GetTask: if the repo says no rows, it's TaskNotFound.
        // If repo returns nil model/nil error, then it's truly no config.
        // Let's assume repo returns sql.ErrNoRows if no config exists.
        // Thus, the TaskNotFound path above handles this. Reaching here means a config *was* found.
	}


	// Convert internal model back to A2A type
	configA2A := models.PushNotificationConfigModelToA2A(configModel) // Need conversion

	return &a2a.TaskPushNotificationConfig{
		ID:                     params.ID,
		PushNotificationConfig: *configA2A,
	}, nil
}

// --- Internal Methods Implementations ---

func (s *taskService) GetTaskByID(ctx context.Context, taskID string) (*models.Task, *a2a.JSONRPCError) {
	taskModel, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, a2a.NewTaskNotFoundError(taskID)
		}
		log.Errorf("Internal error finding task %s: %v", taskID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to retrieve task data"})
	}
	return taskModel, nil
}

func (s *taskService) CreateTask(ctx context.Context, taskData *models.Task) (*models.Task, *a2a.JSONRPCError) {
    // Add initial history record
    if taskData.Status.State != "" { // Only add if status is initialized
        if historyErr := s.AddTaskHistory(ctx, taskData.ID, models.TaskStatusModelToA2A(taskData.Status)); historyErr != nil {
            log.Errorf("Failed to add initial history for task %s: %v", taskData.ID, historyErr)
            // Continue creation, but log the error
        }
    }

	createdTask, err := s.repo.Create(ctx, taskData)
	if err != nil {
		// Handle potential duplicate key errors etc.
		log.Errorf("Internal error creating task %s: %v", taskData.ID, err)
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to create task record"})
	}
	return createdTask, nil
}

func (s *taskService) UpdateTask(ctx context.Context, taskData *models.Task) (*models.Task, *a2a.JSONRPCError) {
    // Add history record for the new status
    if taskData.Status.State != "" {
        if historyErr := s.AddTaskHistory(ctx, taskData.ID, models.TaskStatusModelToA2A(taskData.Status)); historyErr != nil {
             log.Errorf("Failed to add update history for task %s: %v", taskData.ID, historyErr)
             // Continue update, but log the error
        }
    }

	updatedTask, err := s.repo.Update(ctx, taskData)
	if err != nil {
		log.Errorf("Internal error updating task %s: %v", taskData.ID, err)
		// Could check for sql.ErrNoRows here if Update should fail on non-existent task
		return nil, a2a.NewInternalError(map[string]string{"details": "failed to update task record"})
	}
	return updatedTask, nil
}

func (s *taskService) AddTaskHistory(ctx context.Context, taskID string, status a2a.TaskStatus) error {
    // TODO: Check if history is enabled globally or per-agent/task
    return s.repo.AddHistory(ctx, taskID, status)
}

func (s *taskService) GetTaskHistory(ctx context.Context, taskID string, limit int) ([]a2a.TaskStatus, error) {
    // TODO: Check if history is enabled
    return s.repo.GetHistory(ctx, taskID, limit)
}


func (s *taskService) GetTargetAgentURLForTask(ctx context.Context, taskID string) (string, *a2a.JSONRPCError) {
	// How is the target agent URL stored? Assume it's part of the Task model for now.
	taskModel, err := s.repo.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", a2a.NewTaskNotFoundError(taskID)
		}
		log.Errorf("Internal error finding task %s for URL lookup: %v", taskID, err)
		return "", a2a.NewInternalError(map[string]string{"details": "failed to retrieve task data for routing"})
	}

	if taskModel.TargetAgentURL == "" { // Assuming TargetAgentURL field exists in models.Task
		log.Errorf("Task %s found but has no target agent URL defined", taskID)
		return "", a2a.NewInternalError(map[string]string{"details": "task configuration error: missing target agent URL"})
	}

	return taskModel.TargetAgentURL, nil
}

// --- Need conversion functions in models package ---
// e.g., models.TaskModelToA2A(*models.Task) *a2a.Task
// e.g., models.TaskA2AToModel(*a2a.Task) *models.Task
// e.g., models.PushNotificationConfigModelToA2A(*models.PushNotificationConfig) *a2a.PushNotificationConfig
// etc.