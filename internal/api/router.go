// internal/api/router.go
package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/kathir-ks/a2a-platform/internal/app"
	"github.com/kathir-ks/a2a-platform/internal/ws"
)

// API struct remains the same...
type API struct {
	platformService app.PlatformService
	taskService     app.TaskService
	wsManager       ws.Manager
	// Add other services (AgentService, ToolService) as needed
}


// NewRouter creates and configures the main application router.
func NewRouter(ps app.PlatformService, ts app.TaskService, wsm ws.Manager) *mux.Router {
	api := &API{
		platformService: ps,
		taskService:     ts,
		wsManager:       wsm,
		// Initialize other services
	}

	router := mux.NewRouter()

	// Apply middleware globally
	// Order matters: Recovery should generally wrap logging and the actual handlers.
	router.Use(recoveryMiddleware)
	router.Use(loggingMiddleware)
	// router.Use(corsMiddleware) // Uncomment and configure if needed

	// Define routes AFTER applying middleware
	a2aSubrouter := router.PathPrefix("/a2a").Subrouter()
	a2aSubrouter.HandleFunc("", api.handleA2ARequest).Methods(http.MethodPost)
	// If you had other /a2a/... routes, they'd go here and inherit middleware

	// WebSocket endpoint (still placeholder)
	// Using PathPrefix allows middleware to apply to the WS route too
	// wsSubrouter := router.PathPrefix("/ws").Subrouter()
	// wsSubrouter.HandleFunc("", api.handleWebSocket) // Add this later

	// Health check endpoint (good practice, bypasses some A2A logic)
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods(http.MethodGet)


	return router
}