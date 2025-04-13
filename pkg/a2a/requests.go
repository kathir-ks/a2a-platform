// pkg/a2a/requests.go
package a2a

// --- Parameter Structs ---

// TaskIdParams is used for requests targeting a specific task ID.
type TaskIdParams struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// TaskQueryParams includes parameters for querying task details.
type TaskQueryParams struct {
	ID            string         `json:"id"`
	HistoryLength *int           `json:"historyLength,omitempty"` // Pointer for optional int
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// TaskSendParams holds parameters for sending a message to a task.
type TaskSendParams struct {
	ID               string                  `json:"id"`       // Task ID
	SessionID        *string                 `json:"sessionId,omitempty"` // Optional session ID
	Message          Message                 `json:"message"`  // The message being sent (required)
	PushNotification *PushNotificationConfig `json:"pushNotification,omitempty"` // Optional push config for this message
	HistoryLength    *int                    `json:"historyLength,omitempty"` // Optional override for history length in response
	Metadata         map[string]any          `json:"metadata,omitempty"`
}


// --- Request Structs ---
// Each request struct embeds JSONRPCRequest and specifies its Params type.

// SendTaskRequest initiates or continues a task with a message.
type SendTaskRequest struct {
	JSONRPCMessage
	Method string         `json:"method"` // Should be MethodSendTask
	Params TaskSendParams `json:"params"`
}

// SendTaskStreamingRequest initiates or continues a task and subscribes to updates.
type SendTaskStreamingRequest struct {
	JSONRPCMessage
	Method string         `json:"method"` // Should be MethodSendTaskSubscribe
	Params TaskSendParams `json:"params"`
}

// GetTaskRequest retrieves the current state of a task.
type GetTaskRequest struct {
	JSONRPCMessage
	Method string          `json:"method"` // Should be MethodGetTask
	Params TaskQueryParams `json:"params"`
}

// CancelTaskRequest requests the cancellation of a task.
type CancelTaskRequest struct {
	JSONRPCMessage
	Method string         `json:"method"` // Should be MethodCancelTask
	Params TaskIdParams   `json:"params"`
}

// TaskResubscriptionRequest re-subscribes to updates for an existing task stream.
type TaskResubscriptionRequest struct {
	JSONRPCMessage
	Method string          `json:"method"` // Should be MethodResubscribeTask
	Params TaskQueryParams `json:"params"`
}

// SetTaskPushNotificationRequest sets the push notification config for a task.
type SetTaskPushNotificationRequest struct {
	JSONRPCMessage
	Method string `json:"method"` // Should be MethodSetTaskPushNotification
	// Params field directly uses TaskPushNotificationConfig according to schema
	Params TaskPushNotificationConfig `json:"params"`
}

// GetTaskPushNotificationRequest retrieves the push notification config for a task.
type GetTaskPushNotificationRequest struct {
	JSONRPCMessage
	Method string       `json:"method"` // Should be MethodGetTaskPushNotification
	Params TaskIdParams `json:"params"`
}

// Note: A2ARequest from the schema (oneOf various requests) is not directly
// represented as a single Go struct. Decoding logic will typically unmarshal
// into JSONRPCRequest first, then switch on the Method field to determine
// the specific request type and unmarshal the Params accordingly.