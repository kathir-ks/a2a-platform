// pkg/a2a/jsonrpc_base.go
package a2a

// JSONRPCMessage is the base for requests and responses.
type JSONRPCMessage struct {
	JSONRPC string `json:"jsonrpc"` // Should be "2.0"
	// ID can be string, number, or null. interface{} handles this.
	// Use pointers (*string, *int) if stricter typing is desired, but interface{} is common.
	ID interface{} `json:"id,omitempty"`
}

// JSONRPCRequest represents a generic JSON-RPC request.
type JSONRPCRequest struct {
	JSONRPCMessage           // Embed base fields (jsonrpc, id)
	Method         string    `json:"method"`
	Params         any       `json:"params,omitempty"` // Use 'any' (interface{}) as params structure varies
}

// JSONRPCError represents the error object in a JSON-RPC response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"` // Can be anything or null
}

// JSONRPCResponse represents a generic JSON-RPC response.
type JSONRPCResponse struct {
	JSONRPCMessage           // Embed base fields (jsonrpc, id)
	Result         any       `json:"result,omitempty"` // Use 'any' as result structure varies
	Error          *JSONRPCError `json:"error,omitempty"`  // Pointer, as it's absent on success
}