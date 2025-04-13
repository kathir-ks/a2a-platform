// pkg/a2a/errors.go
package a2a

import "fmt"

// Standard JSON-RPC Error Codes
const (
	CodeParseError          = -32700
	CodeInvalidRequest      = -32600
	CodeMethodNotFound      = -32601
	CodeInvalidParams       = -32602
	CodeInternalError       = -32603
	// -32000 to -32099 are reserved for implementation-defined server-errors.
	CodeTaskNotFound                = -32001
	CodeTaskNotCancelable           = -32002
	CodePushNotificationNotSupported = -32003
	CodeUnsupportedOperation         = -32004
)

// --- Predefined Error Structures (matching schema) ---
// These can be useful for generating consistent error responses.

func NewJSONRPCError(code int, message string, data any) *JSONRPCError {
	return &JSONRPCError{Code: code, Message: message, Data: data}
}

func NewParseError(data any) *JSONRPCError {
	return NewJSONRPCError(CodeParseError, "Invalid JSON payload", data)
}

func NewInvalidRequestError(data any) *JSONRPCError {
	return NewJSONRPCError(CodeInvalidRequest, "Request payload validation error", data)
}

func NewMethodNotFoundError(data any) *JSONRPCError {
	return NewJSONRPCError(CodeMethodNotFound, "Method not found", data) // Spec says data should be null, but we allow flexibility
}

func NewInvalidParamsError(data any) *JSONRPCError {
	return NewJSONRPCError(CodeInvalidParams, "Invalid parameters", data)
}

func NewInternalError(data any) *JSONRPCError {
	return NewJSONRPCError(CodeInternalError, "Internal error", data)
}

// --- A2A Specific Errors ---

func NewTaskNotFoundError(taskId string) *JSONRPCError {
	return NewJSONRPCError(CodeTaskNotFound, "Task not found", map[string]string{"taskId": taskId}) // Add task ID to data
}

func NewTaskNotCancelableError(taskId string) *JSONRPCError {
	return NewJSONRPCError(CodeTaskNotCancelable, "Task cannot be canceled", map[string]string{"taskId": taskId})
}

func NewPushNotificationNotSupportedError() *JSONRPCError {
	return NewJSONRPCError(CodePushNotificationNotSupported, "Push Notification is not supported", nil)
}

func NewUnsupportedOperationError(operation string) *JSONRPCError {
	return NewJSONRPCError(CodeUnsupportedOperation, "This operation is not supported", map[string]string{"operation": operation})
}

// Error makes JSONRPCError satisfy the error interface
func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}