// internal/api/handlers.go
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus" // Or your preferred logger
)

// handleA2ARequest handles incoming JSON-RPC requests on the /a2a endpoint.
func (api *API) handleA2ARequest(w http.ResponseWriter, r *http.Request) {
	var response *a2a.JSONRPCResponse
	var request a2a.JSONRPCRequest // Use generic request first
	requestID := interface{}(nil) // Keep track of the request ID

	// 1. Decode Request Body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read request body: %v", err)
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          a2a.NewInternalError("failed to read request body"),
		}
		writeJSONResponse(w, http.StatusInternalServerError, response) // Use status 500 for read error
		return
	}
	defer r.Body.Close()

	// Use json.RawMessage for Params initially to allow specific decoding later
	var rawRequest struct {
		a2a.JSONRPCMessage
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"` // Decode params lazily
	}

	if err := json.Unmarshal(body, &rawRequest); err != nil {
		log.Warnf("Failed to unmarshal JSON-RPC request: %v", err)
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID}, // ID might be absent/invalid here
			Error:          a2a.NewParseError(err.Error()),
		}
		// Try to get ID if possible, even from partial parse (best effort)
		_ = json.Unmarshal(body, &request)
		if request.ID != nil {
			response.ID = request.ID
		}
		writeJSONResponse(w, http.StatusBadRequest, response) // Use status 400 for parse error
		return
	}

	requestID = rawRequest.ID // Store the valid ID

	// 2. Basic Validation
	if rawRequest.JSONRPC != a2a.JSONRPCVersion {
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          a2a.NewInvalidRequestError("invalid jsonrpc version"),
		}
		writeJSONResponse(w, http.StatusBadRequest, response)
		return
	}
	if rawRequest.Method == "" {
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          a2a.NewInvalidRequestError("method is required"),
		}
		writeJSONResponse(w, http.StatusBadRequest, response)
		return
	}

	// 3. Route to appropriate service based on method
	var result any
	var rpcErr *a2a.JSONRPCError

	ctx := r.Context() // Pass context down

	switch rawRequest.Method {
	case a2a.MethodSendTask:
		var params a2a.TaskSendParams
		if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
			// --- Call Platform Service ---
			// We assume PlatformService handles the orchestration:
			// - finding the agent
			// - creating/updating task in TaskService
			// - calling the external agent via agentruntime client
			// - returning the final task state or an error
			var taskResult *a2a.Task
			taskResult, rpcErr = api.platformService.HandleSendTask(ctx, params)
			result = taskResult // Assign to generic result
		}

	case a2a.MethodGetTask:
		var params a2a.TaskQueryParams
		if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
			// --- Call Task Service ---
			var taskResult *a2a.Task
			taskResult, rpcErr = api.taskService.HandleGetTask(ctx, params)
			result = taskResult
		}

	case a2a.MethodCancelTask:
		var params a2a.TaskIdParams
		if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
			// --- Call Task Service ---
			var taskResult *a2a.Task
			taskResult, rpcErr = api.taskService.HandleCancelTask(ctx, params)
			result = taskResult
		}

	case a2a.MethodSetTaskPushNotification:
		var params a2a.TaskPushNotificationConfig // Note: Params *is* the config object directly
		if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
			// --- Call Task Service ---
			var configResult *a2a.TaskPushNotificationConfig
			configResult, rpcErr = api.taskService.HandleSetTaskPushNotification(ctx, params)
			result = configResult
		}

	case a2a.MethodGetTaskPushNotification:
		var params a2a.TaskIdParams
		if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
			// --- Call Task Service ---
			var configResult *a2a.TaskPushNotificationConfig
			configResult, rpcErr = api.taskService.HandleGetTaskPushNotification(ctx, params)
			result = configResult
		}

		// --- Add cases for other non-streaming methods if any ---

	default:
		log.Warnf("Method not found: %s", rawRequest.Method)
		rpcErr = a2a.NewMethodNotFoundError(nil)
	}

	// 4. Construct and Encode Response
	response = &a2a.JSONRPCResponse{
		JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
		Error:          rpcErr, // Assign error if one occurred
	}
	if rpcErr == nil {
		response.Result = result // Assign result only if no error
	}

	// Determine appropriate HTTP status code based on JSON-RPC error
	httpStatusCode := http.StatusOK // Default success
	if rpcErr != nil {
		httpStatusCode = mapJSONRPCErrorToHTTPStatus(rpcErr.Code)
	}

	writeJSONResponse(w, httpStatusCode, response)
}

// decodeParams tries to unmarshal the raw params into the target struct.
// Returns a JSONRPCError if decoding fails.
func decodeParams(rawParams json.RawMessage, target interface{}) *a2a.JSONRPCError {
	if len(rawParams) == 0 {
		// Handle cases where params might be optional or not provided correctly
		// Depending on the method, empty params might be valid or invalid.
		// For simplicity here, we assume they are required if target is non-nil.
		// Specific handlers might need more nuanced checks.
		// Let's treat empty params as invalid for now if target expects data.
		// You might need to adjust this based on specific method requirements.
        if target != nil {
             // Check if target is a pointer to a struct and if the underlying struct is empty.
             // This logic can get complex. A simpler approach might be needed if methods
             // truly have optional params objects.
             // For now, assume if params are expected, they shouldn't be empty raw message.
		    return a2a.NewInvalidParamsError("parameters are required but missing")
        }
        return nil // Params are optional and missing, which is okay
	}

	if err := json.Unmarshal(rawParams, target); err != nil {
		log.Warnf("Failed to unmarshal params: %v", err)
		// Provide more context in the error data if possible
		return a2a.NewInvalidParamsError(map[string]string{"details": err.Error()})
	}
	return nil
}

// writeJSONResponse encodes the response and writes it to the ResponseWriter.
func writeJSONResponse(w http.ResponseWriter, httpStatusCode int, response *a2a.JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Failed to write JSON response: %v", err)
		// Can't write anymore if Encode failed, just log it.
	}
}

// mapJSONRPCErrorToHTTPStatus maps standard JSON-RPC error codes to appropriate HTTP status codes.
func mapJSONRPCErrorToHTTPStatus(code int) int {
	switch code {
	case a2a.CodeParseError:
		return http.StatusBadRequest
	case a2a.CodeInvalidRequest:
		return http.StatusBadRequest
	case a2a.CodeMethodNotFound:
		return http.StatusNotFound
	case a2a.CodeInvalidParams:
		return http.StatusBadRequest
	case a2a.CodeInternalError:
		return http.StatusInternalServerError
	// A2A Specific errors - map as appropriate
	case a2a.CodeTaskNotFound:
		return http.StatusNotFound // Task not found -> 404
	case a2a.CodeTaskNotCancelable:
		return http.StatusConflict // Cannot perform action due to state -> 409
	case a2a.CodePushNotificationNotSupported:
		return http.StatusNotImplemented // Feature not available -> 501
	case a2a.CodeUnsupportedOperation:
		return http.StatusNotImplemented // Feature not available -> 501
	default:
		// For custom or unknown errors, Internal Server Error is safest
		return http.StatusInternalServerError
	}
}

// --- WebSocket Handler (Placeholder) ---
// func (api *API) handleWebSocket(w http.ResponseWriter, r *http.Request) {
//    // WebSocket upgrade logic
//    // Connection registration with api.wsManager
//    // Goroutine for reading messages (sendSubscribe, resubscribe)
//    // Error handling and connection cleanup
//    log.Info("WebSocket connection attempt") // Placeholder
//    http.Error(w, "WebSocket endpoint not implemented", http.StatusNotImplemented)
// }