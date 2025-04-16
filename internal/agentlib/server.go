// internal/agentlib/server.go
package agentlib

import (
	// "context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

// agentServer wraps the agent's core logic for HTTP handling.
type agentServer struct {
	agent *Agent // Reference back to the main agent struct
}

// NewRouter creates the HTTP router for the agent.
func (ags *agentServer) NewRouter() *mux.Router {
	router := mux.NewRouter()
	// Ensure endpoint path starts with /
	endpointPath := ags.agent.config.EndpointPath
	if !strings.HasPrefix(endpointPath, "/") {
		endpointPath = "/" + endpointPath
	}

	router.HandleFunc(endpointPath, ags.handleA2ARequest).Methods(http.MethodPost)

	// Optional: Add a health check endpoint for the agent itself
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods(http.MethodGet)

	// Apply middleware (logging, recovery - can be simpler than platform's)
	router.Use(loggingMiddleware)
	router.Use(recoveryMiddleware)

	return router
}

// handleA2ARequest handles incoming JSON-RPC requests to the agent.
// This is simplified compared to the platform's handler.
func (ags *agentServer) handleA2ARequest(w http.ResponseWriter, r *http.Request) {
	var response *a2a.JSONRPCResponse
	requestID := interface{}(nil) // Keep track of the request ID

	// 1. Decode Request Body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Agent: Failed to read request body: %v", err)
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          NewInternalError("failed to read request body"),
		}
		writeJSONResponse(w, http.StatusInternalServerError, response)
		return
	}
	defer r.Body.Close()

	// Use json.RawMessage for Params initially
	var rawRequest struct {
		a2a.JSONRPCMessage
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(body, &rawRequest); err != nil {
		log.Warnf("Agent: Failed to unmarshal JSON-RPC request: %v", err)
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          NewParseError(err.Error()),
		}
		// Try to get ID if possible
		var idFinder struct { ID interface{} `json:"id"`}
		_ = json.Unmarshal(body, &idFinder)
		if idFinder.ID != nil { response.ID = idFinder.ID }

		writeJSONResponse(w, http.StatusBadRequest, response)
		return
	}

	requestID = rawRequest.ID // Store the valid ID

	// 2. Basic Validation
	if rawRequest.JSONRPC != a2a.JSONRPCVersion {
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          NewInvalidRequestError("invalid jsonrpc version"),
		}
		writeJSONResponse(w, http.StatusBadRequest, response)
		return
	}
	if rawRequest.Method == "" {
		response = &a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
			Error:          NewInvalidRequestError("method is required"),
		}
		writeJSONResponse(w, http.StatusBadRequest, response)
		return
	}

	// 3. Route to appropriate handler
	var result any
	var rpcErr *a2a.JSONRPCError
	ctx := r.Context() // Pass context down

	switch rawRequest.Method {
	case a2a.MethodSendTask:
		var params a2a.TaskSendParams
		if rpcErr = decodeParams(rawRequest.Params, &params); rpcErr == nil {
			// --- Call the agent's registered TaskHandler ---
			if ags.agent.taskHandler == nil {
				log.Error("Agent: No TaskHandler configured!")
				rpcErr = NewInternalError("agent not configured to handle tasks")
			} else {
				result, rpcErr = ags.agent.taskHandler.HandleTaskSend(ctx, params)
			}
		}

	// --- Add cases for other methods if the TaskHandler supports them ---
	// case a2a.MethodGetTask:
	//     var params a2a.TaskQueryParams
	//     if rpcErr = decodeParams(rawRequest.Params, ¶ms); rpcErr == nil {
	//         if handler, ok := ags.agent.taskHandler.(interface{ HandleGetTask(...) }); ok {
	//             result, rpcErr = handler.HandleGetTask(ctx, params)
	//         } else {
	//              rpcErr = NewMethodNotFoundError("tasks/get not supported by this agent")
	//         }
	//     }
	// case a2a.MethodCancelTask:
	//      ... similar logic ...

	default:
		log.Warnf("Agent: Method not found: %s", rawRequest.Method)
		rpcErr = NewMethodNotFoundError(nil)
	}

	// 4. Construct and Encode Response
	response = &a2a.JSONRPCResponse{
		JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: requestID},
		Error:          rpcErr, // Assign error if one occurred
	}
	if rpcErr == nil {
		response.Result = result // Assign result only if no error
	}

	httpStatusCode := http.StatusOK // Default success
	if rpcErr != nil {
		httpStatusCode = mapJSONRPCErrorToHTTPStatus(rpcErr.Code)
	}

	writeJSONResponse(w, httpStatusCode, response)
}

// decodeParams tries to unmarshal the raw params into the target struct.
func decodeParams(rawParams json.RawMessage, target interface{}) *a2a.JSONRPCError {
	if len(rawParams) == 0 {
		// Generally, A2A methods require params. Check specific method needs if allowing optional.
		return NewInvalidParamsError("parameters are required but missing")
	}
	if err := json.Unmarshal(rawParams, target); err != nil {
		log.Warnf("Agent: Failed to unmarshal params: %v", err)
		return NewInvalidParamsError(map[string]string{"details": err.Error()})
	}
	return nil
}

// writeJSONResponse encodes the response and writes it to the ResponseWriter.
func writeJSONResponse(w http.ResponseWriter, httpStatusCode int, response *a2a.JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Errorf("Agent: Failed to write JSON response: %v", err)
	}
}

// mapJSONRPCErrorToHTTPStatus maps standard JSON-RPC error codes to HTTP status codes.
func mapJSONRPCErrorToHTTPStatus(code int) int {
	// Reusing the same mapping logic as the platform
	switch code {
	case a2a.CodeParseError, a2a.CodeInvalidRequest, a2a.CodeInvalidParams:
		return http.StatusBadRequest
	case a2a.CodeMethodNotFound:
		return http.StatusNotFound
	case a2a.CodeInternalError:
		return http.StatusInternalServerError
	case a2a.CodeTaskNotFound:
		return http.StatusNotFound
	case a2a.CodeTaskNotCancelable:
		return http.StatusConflict
	case a2a.CodePushNotificationNotSupported, a2a.CodeUnsupportedOperation:
		return http.StatusNotImplemented
	default:
		// For custom or unknown errors, Internal Server Error is safest
		return http.StatusInternalServerError
	}
}

// --- Basic Middleware (can be enhanced) ---

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Infof("Agent: Received request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Infof("Agent: Handled request: %s %s in %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.WithField("panic", err).Error("Agent: Recovered from handler panic")
				response := &a2a.JSONRPCResponse{
					JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: nil}, // ID might not be available
					Error:          NewInternalError("agent encountered an unexpected error"),
				}
				writeJSONResponse(w, http.StatusInternalServerError, response)
			}
		}()
		next.ServeHTTP(w, r)
	})
}