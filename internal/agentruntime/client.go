// internal/agentruntime/client.go
package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kathir-ks/a2a-platform/internal/config" // Need config for timeout
	"github.com/kathir-ks/a2a-platform/pkg/a2a"         // Need A2A request types
	log "github.com/sirupsen/logrus"
)

// Client defines the interface for communicating with external A2A agents.
type Client interface {
	// SendRequest sends a standard A2A JSON-RPC request to the target agent.
	// It returns the raw response body on success (HTTP 2xx).
	// It returns an error for network issues, timeouts, or non-2xx HTTP status codes.
	// JSON-RPC level errors within a successful HTTP response should be handled by the caller.
	SendRequest(ctx context.Context, targetAgentURL string, request *a2a.JSONRPCRequest) ([]byte, error)

	// TODO: Add SendStreamingRequest for WebSocket communication later
	// SendStreamingRequest(ctx context.Context, targetAgentURL string, request *a2a.JSONRPCRequest) (connection, error)
}

// httpClient implements the Client interface using Go's standard HTTP client.
type httpClient struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPClient creates a new agent runtime client using HTTP.
func NewHTTPClient(cfg *config.Config) Client {
	if cfg == nil {
		log.Fatal("Cannot create agent runtime client without configuration")
	}
	// Create a shared HTTP client with connection pooling
	// Configure timeouts and other transport settings as needed
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		// Add TLS config if needed for custom CAs etc.
		// TLSClientConfig: &tls.Config{...}
	}

	return &httpClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.AgentCallTimeout, // Set default timeout on the client itself
		},
		timeout: cfg.AgentCallTimeout, // Store for potential context override if needed
	}
}

// SendRequest implements the Client interface.
func (c *httpClient) SendRequest(ctx context.Context, targetAgentURL string, request *a2a.JSONRPCRequest) ([]byte, error) {
	// 1. Marshal the request payload
	requestBytes, err := json.Marshal(request)
	if err != nil {
		log.Errorf("Failed to marshal A2A request for agent %s: %v", targetAgentURL, err)
		return nil, fmt.Errorf("failed to marshal request: %w", err) // Wrap error
	}

	// 2. Create HTTP Request
	// Use the context passed in, the http.Client's timeout will still apply
	// unless the context has an earlier deadline.
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, targetAgentURL, bytes.NewBuffer(requestBytes))
	if err != nil {
		log.Errorf("Failed to create HTTP request for agent %s: %v", targetAgentURL, err)
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	// 3. Set Headers
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	// TODO: Add Authentication headers if needed based on agent's AgentCard.Authentication
	// Example: httpRequest.Header.Set("Authorization", "Bearer "+token)

	log.Debugf("Sending A2A request (ID: %v, Method: %s) to %s", request.ID, request.Method, targetAgentURL)

	// 4. Execute Request
	httpResponse, err := c.client.Do(httpRequest)
	if err != nil {
		// Handle client-level errors (network, DNS, context deadline exceeded etc.)
		log.Warnf("HTTP client error sending request to %s: %v", targetAgentURL, err)
		return nil, fmt.Errorf("http client error calling agent %s: %w", targetAgentURL, err)
	}
	defer httpResponse.Body.Close()

	// 5. Check HTTP Status Code
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		// Read some of the body for context, but don't necessarily trust it's JSON
		bodyBytes, _ := io.ReadAll(io.LimitReader(httpResponse.Body, 1024)) // Limit read size
		errorMsg := fmt.Sprintf("agent %s returned non-2xx status: %d %s",
			targetAgentURL,
			httpResponse.StatusCode,
			http.StatusText(httpResponse.StatusCode),
		)
		if len(bodyBytes) > 0 {
			errorMsg += fmt.Sprintf(" (body preview: %s)", string(bodyBytes))
		}
		log.Warn(errorMsg)
		return nil, fmt.Errorf(errorMsg) // Return a distinct error for non-2xx status
	}

	// 6. Read Response Body
	responseBytes, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		log.Errorf("Failed to read response body from agent %s: %v", targetAgentURL, err)
		return nil, fmt.Errorf("failed to read response body from agent %s: %w", targetAgentURL, err)
	}

	log.Debugf("Received successful HTTP response (%d) from %s (Body size: %d)", httpResponse.StatusCode, targetAgentURL, len(responseBytes))

	// 7. Return raw bytes (caller will decode JSON-RPC structure)
	return responseBytes, nil
}