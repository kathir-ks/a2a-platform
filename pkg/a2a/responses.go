// pkg/a2a/responses.go
package a2a

// Base response structure is JSONRPCResponse

// SendTaskResponse is the response to a SendTaskRequest.
type SendTaskResponse struct {
	JSONRPCMessage
	Result *Task         `json:"result,omitempty"` // Pointer because it can be null on error
	Error  *JSONRPCError `json:"error,omitempty"`
}

// GetTaskResponse is the response to a GetTaskRequest.
type GetTaskResponse struct {
	JSONRPCMessage
	Result *Task         `json:"result,omitempty"`
	Error  *JSONRPCError `json:"error,omitempty"`
}

// CancelTaskResponse is the response to a CancelTaskRequest.
type CancelTaskResponse struct {
	JSONRPCMessage
	Result *Task         `json:"result,omitempty"` // Returns the task state after cancellation attempt
	Error  *JSONRPCError `json:"error,omitempty"`
}

// SetTaskPushNotificationResponse is the response to SetTaskPushNotificationRequest.
type SetTaskPushNotificationResponse struct {
	JSONRPCMessage
	Result *TaskPushNotificationConfig `json:"result,omitempty"` // Returns the config that was set
	Error  *JSONRPCError               `json:"error,omitempty"`
}

// GetTaskPushNotificationResponse is the response to GetTaskPushNotificationRequest.
type GetTaskPushNotificationResponse struct {
	JSONRPCMessage
	Result *TaskPushNotificationConfig `json:"result,omitempty"`
	Error  *JSONRPCError               `json:"error,omitempty"`
}

// SendTaskStreamingResponse represents messages sent *from* the server *to* the client
// over a WebSocket connection established by SendTaskStreamingRequest or TaskResubscriptionRequest.
// Note: This is NOT a direct reply to the initial HTTP/WS upgrade request, but subsequent messages.
// The Result can be a TaskStatusUpdateEvent or TaskArtifactUpdateEvent.
type SendTaskStreamingResponse struct {
	JSONRPCMessage // Often notifications might omit ID, but spec allows it
	Result any           `json:"result,omitempty"` // Use any for TaskStatusUpdateEvent or TaskArtifactUpdateEvent
	Error  *JSONRPCError `json:"error,omitempty"`  // For streaming errors related to the subscription
}