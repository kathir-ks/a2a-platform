// internal/llmclient/gemini_client.go
package llmclient

import (
	"context"
	"errors"
	"fmt"
	"io" // Needed for stream EOF check

	"github.com/google/generative-ai-go/genai"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type geminiClient struct {
	client *genai.Client
	apiKey string // Keep for potential reference, though client uses it internally
}

// NewGeminiClient creates a client for Google Gemini models.
func NewGeminiClient(apiKey string) (Client, error) {
	if apiKey == "" {
		return nil, errors.New("Google Gemini API key is required")
	}
	ctx := context.Background() // Use background context for initialization
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Errorf("Failed to create Gemini client: %v", err)
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &geminiClient{
		client: client,
		apiKey: apiKey,
	}, nil
}

func (c *geminiClient) ProviderName() string {
	return "google" // Or "gemini" - using "google" is common
}

func (c *geminiClient) Generate(ctx context.Context, params GenerationParams) (*GenerationResponse, error) {
	if params.Model == "" {
		return nil, errors.New("model name is required for Gemini generate")
	}
	log.Infof("Calling Gemini Generate (Model: %s)...", params.Model)

	model := c.client.GenerativeModel(params.Model)
	applyGenerationSettings(model, params) // Apply Temperature, MaxTokens etc.

	// Map messages to genai.Content
	genaiParts, err := mapMessagesToGenaiParts(params)
	if err != nil {
		return nil, fmt.Errorf("failed to map messages for Gemini: %w", err)
	}

	resp, err := model.GenerateContent(ctx, genaiParts...)
	if err != nil {
		log.Errorf("Gemini API GenerateContent failed: %v", err)
		return nil, fmt.Errorf("gemini API error: %w", err)
	}

	return mapGenaiResponseToLLMResponse(resp, params.Model)
}

func (c *geminiClient) GenerateStream(ctx context.Context, params GenerationParams) (<-chan GenerationStreamEvent, error) {
	if params.Model == "" {
		return nil, errors.New("model name is required for Gemini generate stream")
	}
	log.Infof("Calling Gemini GenerateStream (Model: %s)...", params.Model)

	model := c.client.GenerativeModel(params.Model)
	applyGenerationSettings(model, params) // Apply Temperature, MaxTokens etc.

	genaiParts, err := mapMessagesToGenaiParts(params)
	if err != nil {
		return nil, fmt.Errorf("failed to map messages for Gemini stream: %w", err)
	}

	stream := model.GenerateContentStream(ctx, genaiParts...)
	resultChan := make(chan GenerationStreamEvent, 10) // Buffered channel

	go func() {
		defer close(resultChan)
		var fullResponseText string
		var lastFinishReason string
		var lastUsage *genai.UsageMetadata

		for {
			resp, err := stream.Next()
			// Check for stream completion or error
			if err == iterator.Done || errors.Is(err, io.EOF) {
				log.Debugf("Gemini stream finished for model %s", params.Model)
				// Send final event with accumulated metadata
				finalEvent := GenerationStreamEvent{
					IsFinal:      true,
					ModelUsed:    &params.Model, // Model used for the request
					FinishReason: &lastFinishReason,
				}
				if lastUsage != nil {
					finalEvent.Usage = map[string]int{
						// Cast int32 to int
						"prompt_tokens":     int(lastUsage.PromptTokenCount),
						"completion_tokens": int(lastUsage.CandidatesTokenCount),
						// "total_tokens":      int(lastUsage.TotalTokenCount), // Can add if needed
					}
				}
				resultChan <- finalEvent
				break // Exit the loop cleanly
			}
			if err != nil {
				log.Errorf("Error reading Gemini stream: %v", err)
				resultChan <- GenerationStreamEvent{Error: fmt.Errorf("stream read error: %w", err)}
				break // Exit the loop on error
			}

			// Process the response chunk
			chunkText, finishReason, usage := extractContentFromGenaiResponse(resp)
			fullResponseText += chunkText // Accumulate text if needed later
			if finishReason != "" {
				lastFinishReason = finishReason
			}
			if usage != nil {
				lastUsage = usage // Store the latest usage data (often comes at the end)
			}

			if chunkText != "" {
				resultChan <- GenerationStreamEvent{
					Chunk: chunkText,
				}
			}
			// Removed invalid character check, assuming it was a copy-paste artifact fixed now
		}
	}()

	return resultChan, nil
}

// --- Helper Functions ---

// applyGenerationSettings configures the genai model based on llmclient params.
func applyGenerationSettings(model *genai.GenerativeModel, params GenerationParams) {
	config := &genai.GenerationConfig{}
	shouldSet := false

	if params.MaxTokens != nil {
		mt := int32(*params.MaxTokens) // genai uses int32
		// Assign the address of mt because MaxOutputTokens is *int32
		config.MaxOutputTokens = &mt
		shouldSet = true
	}
	if params.Temperature != nil {
		t := float32(*params.Temperature) // genai uses float32
		// Assign the address of t because Temperature is *float32
		config.Temperature = &t
		shouldSet = true
	}
	if len(params.StopSequences) > 0 {
		config.StopSequences = params.StopSequences
		shouldSet = true
	}
	// Add TopP, TopK if needed and supported by genai.GenerationConfig

	if shouldSet {
		model.GenerationConfig = *config
	}
}

// mapMessagesToGenaiParts converts llmclient messages to genai Content parts.
// Handles system prompts by potentially prepending them.
func mapMessagesToGenaiParts(params GenerationParams) ([]genai.Part, error) {
	var parts []genai.Part

	// Handle System Prompt (Gemini often takes it as the first 'user' message potentially, or API specific field)
	// For simple chat, let's prepend if present. Check Gemini API docs for best practice.
	// NOTE: Gemini API might prefer system instructions within the first message or specific fields not used here yet.
	// Let's prepend it to the first user message for now, or send as a separate initial message if that's better.
	// Simpler: Add to history if using chat session, or just include in parts for GenerateContent.
	if params.SystemPrompt != nil && *params.SystemPrompt != "" {
		// Prepending as a text part. Roles aren't directly associated with individual parts in GenerateContent.
		// The sequence implies the conversation turn.
		// parts = append(parts, genai.Text(*params.SystemPrompt)) // Maybe needs role hints?
		log.Warn("Gemini client currently handles system prompt by simple inclusion; behavior may vary.")
		// Let's try adding it as the very first part.
		parts = append(parts, genai.Text(*params.SystemPrompt))
	}

	for _, msg := range params.Messages {
		// Map roles: llmclient uses "assistant", genai uses "model"
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		if role != "user" && role != "model" {
			log.Warnf("Unsupported role '%s' in message for Gemini, skipping.", msg.Role)
			continue
		}
		// Gemini GenerateContent API takes a flat list of Parts. Role is implied by sequence.
		// The ChatSession API uses explicit roles. Since we use GenerateContent, we just append text.
		parts = append(parts, genai.Text(msg.Content))
	}
	if len(parts) == 0 {
		return nil, errors.New("no valid messages found to send")
	}
	return parts, nil
}

// mapGenaiResponseToLLMResponse converts the non-streaming response.
func mapGenaiResponseToLLMResponse(resp *genai.GenerateContentResponse, requestedModel string) (*GenerationResponse, error) {
	content, finishReason, usage := extractContentFromGenaiResponse(resp)

	llmResp := &GenerationResponse{
		Content:      content,
		ModelUsed:    requestedModel, // Gemini response doesn't always echo back the exact model string
		FinishReason: finishReason,
	}

	if usage != nil {
		llmResp.Usage = map[string]int{
			// Cast int32 to int
			"prompt_tokens":     int(usage.PromptTokenCount),
			"completion_tokens": int(usage.CandidatesTokenCount),
			// "total_tokens":      int(usage.TotalTokenCount),
		}
	}

	return llmResp, nil
}

// extractContentFromGenaiResponse extracts text, finish reason, and usage from a response object.
func extractContentFromGenaiResponse(resp *genai.GenerateContentResponse) (text string, finishReason string, usage *genai.UsageMetadata) {
	if resp == nil {
		return "", "", nil
	}
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					text += string(txt)
				}
			}
		}
		// Use the finish reason from the first candidate (usually only one)
		if finishReason == "" && cand.FinishReason != genai.FinishReasonUnspecified {
			finishReason = cand.FinishReason.String()
		}
	}

	// Extract Usage Metadata if available (usually on the last chunk in streaming OR at the end of non-streaming)
	if resp.UsageMetadata != nil {
		usage = resp.UsageMetadata
		// Don't attempt to get finish reason from UsageMetadata, it's not there
	}

	// Default finish reason if still unspecified (e.g., if only UsageMetadata was present without FinishReason)
	if finishReason == "" {
		// Check if the response might indicate an error/blockage instead of normal stop
		if resp.PromptFeedback != nil && resp.PromptFeedback.BlockReason != genai.BlockReasonUnspecified {
            finishReason = resp.PromptFeedback.BlockReason.String()
        } else {
			// Assume normal stop otherwise
			finishReason = "stop"
		}
	}

	return text, finishReason, usage
}