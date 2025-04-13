// pkg/a2a/schema_types.go
package a2a

import "time"

// --- Agent Definition Types ---

// AgentProvider describes the provider of the agent.
type AgentProvider struct {
	Organization string  `json:"organization"`
	URL          *string `json:"url,omitempty"` // Optional field, use pointer
}

// AgentCapabilities describes the features supported by the agent.
type AgentCapabilities struct {
	Streaming             bool `json:"streaming,omitempty"` // Default is false
	PushNotifications     bool `json:"pushNotifications,omitempty"` // Default is false
	StateTransitionHistory bool `json:"stateTransitionHistory,omitempty"` // Default is false
}

// AgentAuthentication describes the authentication methods supported or required by the agent.
type AgentAuthentication struct {
	Schemes     []string `json:"schemes"`               // Required
	Credentials *string  `json:"credentials,omitempty"` // Optional credential info/placeholder
}

// AgentSkill describes a specific capability or function of the agent.
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`  // Null or array -> slice
	OutputModes []string `json:"outputModes,omitempty"` // Null or array -> slice
}

// AgentCard is the main description of an agent.
type AgentCard struct {
	Name               string               `json:"name"`
	Description        *string              `json:"description,omitempty"`
	URL                string               `json:"url"` // Agent's endpoint URL
	Provider           *AgentProvider       `json:"provider,omitempty"`
	Version            string               `json:"version"`
	DocumentationURL   *string              `json:"documentationUrl,omitempty"`
	Capabilities       AgentCapabilities    `json:"capabilities"` // Required, struct is not nullable
	Authentication     *AgentAuthentication `json:"authentication,omitempty"`
	DefaultInputModes  []string             `json:"defaultInputModes,omitempty"` // Default handled by consumer if empty
	DefaultOutputModes []string             `json:"defaultOutputModes,omitempty"` // Default handled by consumer if empty
	Skills             []AgentSkill         `json:"skills"`                     // Required
}

// --- Message and Content Types ---

// FileContent represents file data, either inline or via URI.
// Validation should ensure only one of Bytes or URI is set.
type FileContent struct {
	Name     *string `json:"name,omitempty"`
	MimeType *string `json:"mimeType,omitempty"`
	Bytes    *string `json:"bytes,omitempty"` // Base64 encoded string
	URI      *string `json:"uri,omitempty"`
}

// Base interface for Message Parts (used for type checking/switching).
// We use map[string]interface{} for Parts within Message/Artifact
// due to the complexity of marshalling/unmarshalling 'anyOf'.
// Actual decoding will require checking the "type" field.
// type Part interface { isPart() } // Marker interface (optional)

// TextPart represents a text segment of a message.
type TextPart struct {
	Type     string         `json:"type"` // Should be PartTypeText ("text")
	Text     string         `json:"text"`
	Metadata map[string]any `json:"metadata,omitempty"`
	// func (TextPart) isPart() {} // Implement marker interface (optional)
}

// FilePart represents a file segment of a message.
type FilePart struct {
	Type     string         `json:"type"` // Should be PartTypeFile ("file")
	File     FileContent    `json:"file"` // Struct, required
	Metadata map[string]any `json:"metadata,omitempty"`
	// func (FilePart) isPart() {} // Implement marker interface (optional)
}

// DataPart represents a structured data segment of a message.
type DataPart struct {
	Type     string         `json:"type"` // Should be PartTypeData ("data")
	Data     map[string]any `json:"data"` // Required
	Metadata map[string]any `json:"metadata,omitempty"`
	// func (DataPart) isPart() {} // Implement marker interface (optional)
}

// Message represents a single message in a task conversation.
type Message struct {
	Role  string `json:"role"` // Required ("user" or "agent")
	// Parts uses interface{} because marshalling requires checking the 'type' field.
	// During unmarshalling, decode into []map[string]interface{}, check 'type',
	// then unmarshal the map into the specific Part struct (TextPart, FilePart, DataPart).
	Parts    []any          `json:"parts"` // Required. Use []any or []map[string]any.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Artifact represents a potentially large or structured output from a task.
type Artifact struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	// Parts uses interface{} for the same reason as Message.Parts
	Parts     []any          `json:"parts"` // Required. Use []any or []map[string]any.
	Index     int            `json:"index,omitempty"`      // Default 0
	Append    *bool          `json:"append,omitempty"`     // Optional bool
	LastChunk *bool          `json:"lastChunk,omitempty"`  // Optional bool for streaming artifacts
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// --- Task Related Types ---

// TaskStatus represents the current status of a task.
type TaskStatus struct {
	State     TaskState  `json:"state"`               // Required
	Message   *Message   `json:"message,omitempty"`   // Optional message associated with the status
	Timestamp *time.Time `json:"timestamp,omitempty"` // Use pointer for optional timestamp
}

// Task represents the state of an ongoing or completed task.
type Task struct {
	ID        string     `json:"id"`
	SessionID *string    `json:"sessionId,omitempty"`
	Status    TaskStatus `json:"status"` // Required
	// Artifacts can be null or an array
	Artifacts []Artifact     `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// --- Push Notification Types ---

// AuthenticationInfo used within PushNotificationConfig.
type AuthenticationInfo struct {
	Schemes     []string       `json:"schemes"`               // Required
	Credentials *string        `json:"credentials,omitempty"` // Optional
	// Note: The schema has `additionalProperties: {}`, implying potential extra fields.
	// If needed, add: Extra map[string]interface{} `json:"-"` and handle custom marshal/unmarshal.
}

// PushNotificationConfig describes how the agent should send push notifications.
type PushNotificationConfig struct {
	URL            string              `json:"url"` // Required
	Token          *string             `json:"token,omitempty"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
}

// TaskPushNotificationConfig associates a PushNotificationConfig with a Task ID.
// Used specifically in the get/set push notification responses.
type TaskPushNotificationConfig struct {
	ID                     string                 `json:"id"` // Task ID
	PushNotificationConfig PushNotificationConfig `json:"pushNotificationConfig"`
}