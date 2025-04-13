// pkg/a2a/constants.go
package a2a

const (
	// JSONRPCVersion specifies the version of the JSON-RPC protocol.
	JSONRPCVersion = "2.0"

	// Method names for A2A requests
	MethodSendTask                   = "tasks/send"
	MethodSendTaskSubscribe          = "tasks/sendSubscribe" // For streaming
	MethodGetTask                    = "tasks/get"
	MethodCancelTask                 = "tasks/cancel"
	MethodResubscribeTask            = "tasks/resubscribe" // For streaming reconnection
	MethodSetTaskPushNotification    = "tasks/pushNotification/set"
	MethodGetTaskPushNotification    = "tasks/pushNotification/get"

	// Message Roles
	RoleUser  = "user"
	RoleAgent = "agent"

	// Part Types
	PartTypeText = "text"
	PartTypeFile = "file"
	PartTypeData = "data"
)