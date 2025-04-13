// internal/tools/llm_tool.go
package tools

import (
	"context"
	"fmt"
	"strings"
	"encoding/json"
	
	"github.com/kathir-ks/a2a-platform/internal/llmclient" // LLM Client interface
	"github.com/kathir-ks/a2a-platform/internal/models"
	log "github.com/sirupsen/logrus"
)

const LLMToolName = "language_model"

// LLMTool provides access to configured language models.
type LLMTool struct {
	clients       map[string]llmclient.Client // Map provider name to client instance
	defaultModel  string                      // Default model identifier (e.g., "openai:gpt-4o")
	defaultProvider string                    // Default provider name extracted from defaultModel
}

var _ Executor = (*LLMTool)(nil) // Compile-time check

// NewLLMTool creates a new LLM tool executor.
// It requires initialized LLM clients and the default model identifier.
func NewLLMTool(clients []llmclient.Client, defaultModelIdentifier string) (*LLMTool, error) {
	if len(clients) == 0 {
		return nil, fmt.Errorf("at least one LLM client must be provided")
	}
	if defaultModelIdentifier == "" {
        return nil, fmt.Errorf("default LLM model identifier is required")
    }


	clientMap := make(map[string]llmclient.Client)
	for _, c := range clients {
        providerName := c.ProviderName()
        if providerName == "" {
            log.Warnf("LLM client provided without a ProviderName, skipping.")
            continue
        }
		if _, exists := clientMap[providerName]; exists {
             log.Warnf("Multiple LLM clients provided for provider '%s', using the last one.", providerName)
        }
		clientMap[providerName] = c
        log.Infof("LLM Tool initialized with client for provider: %s", providerName)
	}

    if len(clientMap) == 0 {
        return nil, fmt.Errorf("no valid LLM clients provided (missing ProviderName?)")
    }

    // Determine default provider
    defaultProvider, _, ok := strings.Cut(defaultModelIdentifier, ":")
    if !ok {
        return nil, fmt.Errorf("invalid default LLM model identifier format (expected 'provider:model'): %s", defaultModelIdentifier)
    }
    if _, providerExists := clientMap[defaultProvider]; !providerExists {
        // Fallback if default provider isn't configured? Or error out? Let's error for now.
         return nil, fmt.Errorf("default LLM provider '%s' specified but no client configured for it", defaultProvider)
    }

	return &LLMTool{
		clients:       clientMap,
		defaultModel:  defaultModelIdentifier,
        defaultProvider: defaultProvider,
	}, nil
}

// GetDefinition implements the tools.Executor interface.
func (t *LLMTool) GetDefinition() models.ToolDefinition {
	// Base schema - could be enhanced dynamically based on available models/providers
	return models.ToolDefinition{
		Name:        LLMToolName,
		Description: "Accesses large language models for text generation or chat.",
		Schema: models.Metadata{
			"type": "object",
			"properties": map[string]any{
				"model": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("Optional. Model identifier (e.g., 'openai:gpt-4o', 'anthropic:claude-3-opus-20240229'). Defaults to '%s'.", t.defaultModel),
				},
				"system_prompt": map[string]any{
					"type":        "string",
					"description": "Optional. System message to guide the model's behavior.",
				},
                "prompt": map[string]any{ // Allow single prompt for simpler cases
					"type":        "string",
					"description": "Optional. A single user prompt. Use 'messages' for chat history.",
				},
				"messages": map[string]any{
					"type": "array",
                    "description": "Optional. A list of messages for chat history. Required if 'prompt' is not provided.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"role":    map[string]any{"type": "string", "enum": []string{"user", "assistant", "system"}},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"role", "content"},
					},
				},
				"max_tokens": map[string]any{
					"type":        "integer",
					"description": "Optional. Maximum number of tokens to generate.",
				},
				"temperature": map[string]any{
					"type":        "number",
					"format":      "float",
                    "minimum": 0.0,
                    "maximum": 2.0,
					"description": "Optional. Sampling temperature (e.g., 0.7). Higher values mean more randomness.",
				},
				"stop_sequences": map[string]any{
					"type":        "array",
                    "items":       map[string]any{"type": "string"},
					"description": "Optional. Sequences where the generation should stop.",
				},
                "stream": map[string]any{
                    "type": "boolean",
                    "default": false,
                    "description": "Optional. Whether to stream the response (requires specific handling by caller).",
                },
			},
			// "required": []string{}, // Logic below determines requirement for prompt/messages
            // --- Output Schema ---
             "output": map[string]any{
                "type": "object",
                "description": "Result for non-streaming calls. Streaming calls provide chunks.",
                "properties": map[string]any{
                    "content": map[string]any{ "type": "string", "description": "The generated text response."},
                    "model_used": map[string]any{ "type": "string", "description": "The specific model identifier used."},
                    "finish_reason": map[string]any{ "type": "string", "description": "Reason generation finished (e.g., 'stop', 'length')."},
                    "usage": map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "prompt_tokens": map[string]any{"type": "integer"},
                            "completion_tokens": map[string]any{"type": "integer"},
                            // "total_tokens": map[string]any{"type": "integer"}, // Optional
                        },
                        "description": "Token usage information.",
                    },
                },
                 "required": []string{"content", "model_used", "finish_reason"},
            },
		},
	}
}

// Execute implements the tools.Executor interface.
// NOTE: This current implementation handles NON-STREAMING requests.
// Handling streaming via a standard tool Execute() is awkward because Execute()
// returns a single result. Streaming might need a different mechanism (e.g., dedicated platform service method).
func (t *LLMTool) Execute(ctx context.Context, params map[string]any) (result map[string]any, err error) {
	llmParams, err := t.parseAndValidateParams(params)
	if err != nil {
		return nil, fmt.Errorf("invalid parameters for %s tool: %w", LLMToolName, err)
	}

    // --- Streaming Check ---
    if llmParams.Stream {
        // How to handle this? The Tool interface expects a single map[string]any return.
        // Option 1: Error out - Simplest, streaming needs different mechanism.
        // Option 2: Return metadata indicating streaming started (awkward).
        // Option 3: Perform the stream internally and aggregate results (defeats purpose of streaming).
        log.Warnf("Streaming requested via tool %s, but standard Execute cannot return a stream. Ignoring stream flag.", LLMToolName)
        // For now, we proceed with a non-streaming call even if stream=true was requested.
        // A better solution involves dedicated streaming support in the platform/A2A protocol.
        llmParams.Stream = false // Force non-streaming for this path
    }

	// Determine client and model
	providerName := t.defaultProvider
	modelNameForCall := ""
    if llmParams.Model != "" { // Model specified in params (e.g., "openai:gpt-4o")
        p, m, ok := strings.Cut(llmParams.Model, ":")
        if !ok || p == "" || m == "" {
            return nil, fmt.Errorf("invalid model format '%s', expected 'provider:model'", llmParams.Model)
        }
        providerName = p     // Use specified provider
        modelNameForCall = m // Use specified model name for the API call
    } else { // Model not specified, use default
        _ , m, ok := strings.Cut(t.defaultModel, ":") // Extract model name from default
        if !ok {
             // This should have been caught during tool initialization, but double-check
             return nil, fmt.Errorf("internal error: invalid default model format '%s'", t.defaultModel)
        }
        modelNameForCall = m
        // providerName remains t.defaultProvider
    }
	
	llmParams.Model = modelNameForCall


	client, ok := t.clients[providerName]
	if !ok {
		return nil, fmt.Errorf("no configured LLM client for provider: %s", providerName)
	}

	log.Infof("Executing LLM tool using provider '%s' and model '%s'", providerName, llmParams.Model)

	// Call the LLM Client (non-streaming)
	resp, err := client.Generate(ctx, llmParams)
	if err != nil {
		log.Errorf("LLM client '%s' failed: %v", providerName, err)
		// Return a structured error potentially? Or just the error message?
		return nil, fmt.Errorf("LLM execution failed (provider: %s): %w", providerName, err)
	}

	// Map response to output schema
	result = map[string]any{
		"content":       resp.Content,
		"model_used":    fmt.Sprintf("%s:%s", providerName, resp.ModelUsed), // Add provider prefix back
		"finish_reason": resp.FinishReason,
		"usage":         resp.Usage, // Pass usage map directly
	}

	return result, nil
}

// parseAndValidateParams converts the generic map to structured llmclient.GenerationParams
func (t *LLMTool) parseAndValidateParams(params map[string]any) (llmclient.GenerationParams, error) {
	var genParams llmclient.GenerationParams

	// --- Model ---
    if modelVal, ok := params["model"]; ok {
        if modelStr, ok := modelVal.(string); ok {
             genParams.Model = modelStr // Keep provider prefix for now
        } else {
            return genParams, fmt.Errorf("'model' must be a string")
        }
    } // Use default if empty

	// --- System Prompt ---
    if spVal, ok := params["system_prompt"]; ok {
        if spStr, ok := spVal.(string); ok {
             genParams.SystemPrompt = &spStr
        } else {
            return genParams, fmt.Errorf("'system_prompt' must be a string")
        }
    }

	// --- Messages / Prompt ---
    hasMessages := false
    if msgVal, ok := params["messages"]; ok {
        // Need robust conversion from []any or []map[string]any to []llmclient.Message
        messagesBytes, err := json.Marshal(msgVal)
        if err != nil { return genParams, fmt.Errorf("cannot marshal 'messages': %w", err)}
        var messages []llmclient.Message
        if err := json.Unmarshal(messagesBytes, &messages); err != nil {
            return genParams, fmt.Errorf("invalid 'messages' format: %w", err)
        }
        if len(messages) > 0 {
            genParams.Messages = messages
            hasMessages = true
        }
    }

    if promptVal, ok := params["prompt"]; ok && !hasMessages {
        if promptStr, ok := promptVal.(string); ok && promptStr != "" {
            genParams.Messages = []llmclient.Message{{Role: "user", Content: promptStr}}
             hasMessages = true
        } else if !hasMessages {
             return genParams, fmt.Errorf("'prompt' must be a non-empty string if 'messages' is not provided")
        }
    }

    if !hasMessages {
         return genParams, fmt.Errorf("either 'prompt' (string) or 'messages' (array) is required")
    }


	// --- Other Parameters (with type checks) ---
    if mtVal, ok := params["max_tokens"]; ok {
		// JSON numbers decode as float64, handle potential conversion
        if mtFloat, ok := mtVal.(float64); ok {
            mtInt := int(mtFloat)
            if float64(mtInt) != mtFloat { // Check for non-integer float
                 return genParams, fmt.Errorf("'max_tokens' must be an integer")
            }
            genParams.MaxTokens = &mtInt
        } else {
            return genParams, fmt.Errorf("'max_tokens' must be an integer")
        }
	}

    if tempVal, ok := params["temperature"]; ok {
         if tempFloat, ok := tempVal.(float64); ok {
            genParams.Temperature = &tempFloat
        } else {
            return genParams, fmt.Errorf("'temperature' must be a number")
        }
    }

     if ssVal, ok := params["stop_sequences"]; ok {
        if ssSlice, ok := ssVal.([]any); ok {
             genParams.StopSequences = make([]string, 0, len(ssSlice))
             for i, item := range ssSlice {
                 if itemStr, ok := item.(string); ok {
                     genParams.StopSequences = append(genParams.StopSequences, itemStr)
                 } else {
                     return genParams, fmt.Errorf("item %d in 'stop_sequences' is not a string", i)
                 }
             }
        } else {
            return genParams, fmt.Errorf("'stop_sequences' must be an array of strings")
        }
    }

    if streamVal, ok := params["stream"]; ok {
        if streamBool, ok := streamVal.(bool); ok {
            genParams.Stream = streamBool
        } else {
             return genParams, fmt.Errorf("'stream' must be a boolean")
        }
    }


	return genParams, nil
}