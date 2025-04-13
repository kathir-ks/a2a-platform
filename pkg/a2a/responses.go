// pkg/a2a/events.go
package a2a

// TaskStatusUpdateEvent is sent during streaming to notify of task status changes.
type TaskStatusUpdateEvent struct {
	ID       string         `json:"id"`      // Task ID
	Status   TaskStatus     `json:"status"`  // The new status
	Final    bool           `json:"final"`   // Indicates if this is the terminal status update
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskArtifactUpdateEvent is sent during streaming when a new artifact or chunk is ready.
type TaskArtifactUpdateEvent struct {
	ID       string         `json:"id"`       // Task ID
	Artifact Artifact       `json:"artifact"` // The artifact data (or chunk)
	Metadata map[string]any `json:"metadata,omitempty"`
}