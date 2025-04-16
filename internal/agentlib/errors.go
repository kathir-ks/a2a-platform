// internal/agentlib/errors.go
package agentlib

import (
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
)

// Convenience functions to create standard A2A errors within agent handlers.

func NewParseError(data any) *a2a.JSONRPCError {
	return a2a.NewParseError(data)
}

func NewInvalidRequestError(data any) *a2a.JSONRPCError {
	return a2a.NewInvalidRequestError(data)
}

func NewMethodNotFoundError(data any) *a2a.JSONRPCError {
	return a2a.NewMethodNotFoundError(data)
}

func NewInvalidParamsError(data any) *a2a.JSONRPCError {
	return a2a.NewInvalidParamsError(data)
}

func NewInternalError(data any) *a2a.JSONRPCError {
	return a2a.NewInternalError(data)
}

func NewTaskNotFoundError(taskId string) *a2a.JSONRPCError {
	return a2a.NewTaskNotFoundError(taskId)
}

func NewTaskNotCancelableError(taskId string) *a2a.JSONRPCError {
	return a2a.NewTaskNotCancelableError(taskId)
}

func NewPushNotificationNotSupportedError() *a2a.JSONRPCError {
	return a2a.NewPushNotificationNotSupportedError()
}

func NewUnsupportedOperationError(operation string) *a2a.JSONRPCError {
	return a2a.NewUnsupportedOperationError(operation)
}