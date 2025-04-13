// internal/repository/memory/task_repo.go
package memory

import (
	"context"
	"encoding/json" // For deep copying via marshal/unmarshal
	"sort"
	"sync"
	"time"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

type memoryTaskRepository struct {
	mu          sync.RWMutex
	tasks       map[string]*models.Task               // taskID -> Task
	pushConfigs map[string]*models.PushNotificationConfig // taskID -> PushConfig
	history     map[string][]a2a.TaskStatus           // taskID -> sorted list of statuses (newest last)
}

// NewMemoryTaskRepository creates a new in-memory task repository.
func NewMemoryTaskRepository() repository.TaskRepository {
	return &memoryTaskRepository{
		tasks:       make(map[string]*models.Task),
		pushConfigs: make(map[string]*models.PushNotificationConfig),
		history:     make(map[string][]a2a.TaskStatus),
	}
}

// deepCopyTask creates a deep copy of a task using JSON marshalling.
// Necessary to prevent external modification of stored data.
func deepCopyTask(original *models.Task) (*models.Task, error) {
	if original == nil {
		return nil, nil
	}
	cpy := &models.Task{}
	bytes, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, cpy)
	if err != nil {
		return nil, err
	}
	return cpy, nil
}

// deepCopyPushConfig creates a deep copy.
func deepCopyPushConfig(original *models.PushNotificationConfig) (*models.PushNotificationConfig, error) {
	if original == nil {
		return nil, nil
	}
	cpy := &models.PushNotificationConfig{}
	bytes, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, cpy)
	if err != nil {
		return nil, err
	}
	return cpy, nil
}


func (r *memoryTaskRepository) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tasks[task.ID]; exists {
		return nil, ErrAlreadyExists
	}

	// Store a deep copy
	taskCopy, err := deepCopyTask(task)
	if err != nil {
        log.Errorf("MemoryRepo: Failed to deep copy task on create %s: %v", task.ID, err)
		return nil, err
	}
    if taskCopy.CreatedAt.IsZero() {
        taskCopy.CreatedAt = time.Now().UTC()
    }
    taskCopy.LastUpdatedAt = taskCopy.CreatedAt

	r.tasks[task.ID] = taskCopy
	log.Debugf("MemoryRepo: Created task %s", task.ID)

    // Return another deep copy
    return deepCopyTask(taskCopy)
}

func (r *memoryTaskRepository) Update(ctx context.Context, task *models.Task) (*models.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existingTask, exists := r.tasks[task.ID]
	if !exists {
		return nil, ErrNotFound
	}

	// Store a deep copy of the new data
	taskCopy, err := deepCopyTask(task)
	if err != nil {
		log.Errorf("MemoryRepo: Failed to deep copy task on update %s: %v", task.ID, err)
		return nil, err
	}
    // Preserve original CreatedAt, update LastUpdatedAt
    taskCopy.CreatedAt = existingTask.CreatedAt
    taskCopy.LastUpdatedAt = time.Now().UTC()

	r.tasks[task.ID] = taskCopy
	log.Debugf("MemoryRepo: Updated task %s", task.ID)

    // Return another deep copy
    return deepCopyTask(taskCopy)
}

func (r *memoryTaskRepository) FindByID(ctx context.Context, id string) (*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	task, exists := r.tasks[id]
	if !exists {
		return nil, ErrNotFound
	}
	log.Debugf("MemoryRepo: Found task %s", id)
    // Return a deep copy
	return deepCopyTask(task)
}

func (r *memoryTaskRepository) FindBySessionID(ctx context.Context, sessionID string) ([]*models.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]*models.Task, 0)
	for _, task := range r.tasks {
		if task.SessionID != nil && *task.SessionID == sessionID {
            // Append a deep copy
            taskCopy, err := deepCopyTask(task)
            if err != nil {
                log.Errorf("MemoryRepo: Failed to deep copy task %s during FindBySessionID: %v", task.ID, err)
                return nil, err // Propagate error if copy fails
            }
			results = append(results, taskCopy)
		}
	}
	log.Debugf("MemoryRepo: Found %d tasks for session %s", len(results), sessionID)
	return results, nil
}

func (r *memoryTaskRepository) SetPushConfig(ctx context.Context, taskID string, config *models.PushNotificationConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure the task itself exists first
	if _, exists := r.tasks[taskID]; !exists {
		return ErrNotFound
	}

    configCopy, err := deepCopyPushConfig(config)
    if err != nil {
         log.Errorf("MemoryRepo: Failed to deep copy push config on set %s: %v", taskID, err)
		return err
    }

	r.pushConfigs[taskID] = configCopy
	log.Debugf("MemoryRepo: Set push config for task %s", taskID)
	return nil
}

func (r *memoryTaskRepository) GetPushConfig(ctx context.Context, taskID string) (*models.PushNotificationConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.pushConfigs[taskID]
	if !exists {
		// Check if the task exists but just has no config - still return ErrNotFound
		// as the config resource itself is not found.
		return nil, ErrNotFound
	}
	log.Debugf("MemoryRepo: Found push config for task %s", taskID)
    // Return a deep copy
	return deepCopyPushConfig(config)
}

func (r *memoryTaskRepository) AddHistory(ctx context.Context, taskID string, status a2a.TaskStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure the task exists
	if _, exists := r.tasks[taskID]; !exists {
		return ErrNotFound
	}

	// Ensure timestamp is set
	if status.Timestamp == nil || status.Timestamp.IsZero() {
        now := time.Now().UTC()
		status.Timestamp = &now
	}

	r.history[taskID] = append(r.history[taskID], status)
    // Optional: Keep history sorted if entries might arrive out of order (unlikely here)
    // sort.SliceStable(r.history[taskID], func(i, j int) bool {
	// 	return r.history[taskID][i].Timestamp.Before(*r.history[taskID][j].Timestamp)
	// })
	log.Debugf("MemoryRepo: Added history entry for task %s (new state: %s)", taskID, status.State)
	return nil
}

func (r *memoryTaskRepository) GetHistory(ctx context.Context, taskID string, limit int) ([]a2a.TaskStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskHistory, exists := r.history[taskID]
	if !exists || len(taskHistory) == 0 {
		return []a2a.TaskStatus{}, nil // Return empty slice, not error
	}

	// Note: History is appended, so newest is last
	if limit <= 0 || limit >= len(taskHistory) {
		// Return full history (or if limit is invalid)
        // Return a copy of the slice to prevent external modification
        histCopy := make([]a2a.TaskStatus, len(taskHistory))
        copy(histCopy, taskHistory)
        log.Debugf("MemoryRepo: Found %d history entries for task %s (returning all)", len(histCopy), taskID)
		return histCopy, nil
	}

	// Return last 'limit' elements
    startIndex := len(taskHistory) - limit
    histCopy := make([]a2a.TaskStatus, limit)
    copy(histCopy, taskHistory[startIndex:])
	log.Debugf("MemoryRepo: Found %d history entries for task %s (returning last %d)", len(taskHistory), limit, taskID)
	return histCopy, nil
}