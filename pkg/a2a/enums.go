// pkg/a2a/enums.go
package a2a

// TaskState represents the different states a task can be in.
type TaskState string

const (
	TaskStateSubmitted    TaskState = "submitted"
	TaskStateWorking      TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateCompleted    TaskState = "completed"
	TaskStateCanceled     TaskState = "canceled"
	TaskStateFailed       TaskState = "failed"
	TaskStateUnknown      TaskState = "unknown" // Good practice for potential future states or errors
)