// internal/api/middleware.go
package api

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus" // Or your preferred logger
)

// loggingMiddleware logs incoming requests.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		lrw := newLoggingResponseWriter(w)

		// Call the next handler in the chain
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		// Log request details
		log.WithFields(log.Fields{
			"method":     r.Method,
			"path":       r.URL.Path,
			"remote_addr": r.RemoteAddr,
			"user_agent": r.UserAgent(),
			"status":     lrw.statusCode,
			"duration_ms": duration.Milliseconds(),
		}).Info("Handled request")
	})
}

// recoveryMiddleware recovers from panics and logs them.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.WithField("panic", err).Error("Recovered from handler panic")

				// Return a generic internal server error response
				// Avoid sending panic details to the client
				response := &a2a.JSONRPCResponse{
					JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: nil}, // ID might not be available here
					Error:          a2a.NewInternalError("an unexpected error occurred"),
				}
				// Attempt to extract ID if request body was parsed before panic
				// This is tricky and might not always work reliably after a panic.
				// Consider how much effort you want to put into ID recovery vs. just logging.

				writeJSONResponse(w, http.StatusInternalServerError, response)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// --- Helper for loggingMiddleware ---

// loggingResponseWriter wraps http.ResponseWriter to capture the status code.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	// Default status code is 200 OK if WriteHeader is not called
	return &loggingResponseWriter{w, http.StatusOK}
}

// WriteHeader captures the status code before writing headers.
func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// Write captures the status code if WriteHeader hasn't been called yet.
// This happens when the first call is Write, which implicitly writes StatusOK.
func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	// The status code is already set by WriteHeader if it was called.
	// If not, the first Write implicitly sets it to 200 OK.
	// We captured the default 200 in newLoggingResponseWriter.
	// WriteHeader call will update it if necessary.
	return lrw.ResponseWriter.Write(b)
}

// --- Example CORS Middleware (Very basic - adjust origins as needed) ---
/*
func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // WARNING: "*" is insecure for production. List specific origins.
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS") // Add PUT, DELETE etc. if needed
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization") // Add custom headers if needed

        // Handle preflight requests
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }

        next.ServeHTTP(w, r)
    })
}
*/