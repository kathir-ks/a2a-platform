// internal/llmclient/interface.go
package llmclient

import (
	"context"
)

// Message represents a single message in a chat history.
type Message struct {
	Role    string `json:"role"` // e.g., "user", "assistant", "system"
	Content string `json:"content"`
}

// GenerationParams holds the parameters for an LLM generation request.
type GenerationParams struct {
	Model         string    `json:"model,omitempty"` // Specific model identifier (e.g., "gpt-4o", "claude-3-opus-20240229")
	SystemPrompt  *string   `json:"system_prompt,omitempty"`
	Messages      []Message `json:"messages"` // Chat history or single prompt
	MaxTokens     *int      `json:"max_tokens,omitempty"`
	Temperature   *float64  `json:"temperature,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
	// Add other common parameters like top_p, top_k if needed
    Stream        bool      `json:"stream,omitempty"` // Indicate if streaming response is desired
}

// GenerationResponse holds the result from a non-streaming LLM call.
type GenerationResponse struct {
	Content      string            `json:"content"`         // The generated text
	ModelUsed    string            `json:"model_used"`      // Actual model that responded
	FinishReason string            `json:"finish_reason"`   // e.g., "stop", "length", "content_filter"
	Usage        map[string]int    `json:"usage,omitempty"` // e.g., {"prompt_tokens": 10, "completion_tokens": 50}
}

// GenerationStreamEvent holds a single event from a streaming LLM call.
type GenerationStreamEvent struct {
    Chunk         string            `json:"chunk,omitempty"`         // Delta content chunk
    IsFinal       bool              `json:"is_final,omitempty"`      // True for the last event in the stream
	ModelUsed     *string           `json:"model_used,omitempty"`    // Often sent with the final chunk
	FinishReason  *string           `json:"finish_reason,omitempty"` // Often sent with the final chunk
	Usage         map[string]int    `json:"usage,omitempty"`         // Often sent with the final chunk
    Error         error             `json:"-"`                       // Internal error during streaming
}


// Client defines the interface for interacting with an LLM provider.
type Client interface {
	// Generate performs a non-streaming text generation request.
	Generate(ctx context.Context, params GenerationParams) (*GenerationResponse, error)

    // GenerateStream performs a streaming text generation request.
    // Returns a channel that emits chunks/events. The channel will be closed
    // when the stream ends or an unrecoverable error occurs.
	GenerateStream(ctx context.Context, params GenerationParams) (<-chan GenerationStreamEvent, error)

    // ProviderName returns the name of the LLM provider (e.g., "openai", "anthropic").
    ProviderName() string
}