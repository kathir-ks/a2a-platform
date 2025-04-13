// internal/models/models.go
package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/kathir-ks/a2a-platform/pkg/a2a" // Need A2A types for conversion
)

// --- Base Enum Types (Internal Representation) ---
// Using the same strings as a2a for simplicity, but could be different
type TaskState string

const (
	TaskStateSubmitted    TaskState = "submitted"
	TaskStateWorking      TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateCompleted    TaskState = "completed"
	TaskStateCanceled     TaskState = "canceled"
	TaskStateFailed       TaskState = "failed"
	TaskStateUnknown      TaskState = "unknown"
)

// --- Core Models ---

// Agent represents an agent registered on the platform.
// Might have DB tags if using an ORM like GORM.
type Agent struct {
	ID                 string    `json:"id" gorm:"primaryKey"` // Platform's internal ID for the agent
	AgentCard          AgentCard `json:"agent_card" gorm:"type:jsonb"` // Store the A2A AgentCard as JSON
	RegisteredAt       time.Time `json:"registered_at" gorm:"autoCreateTime"`
	LastUpdatedAt      time.Time `json:"last_updated_at" gorm:"autoUpdateTime"`
	OwnerID            string    `json:"owner_id,omitempty"` // Optional: User ID who registered it
	IsEnabled          bool      `json:"is_enabled"`
	// Add other platform-specific fields if needed
}

// Task represents the internal state of a task.
type Task struct {
	ID               string               `json:"id" gorm:"primaryKey"` // A2A Task ID
	SessionID        *string              `json:"session_id,omitempty" gorm:"index"`
	Status           TaskStatus           `json:"status" gorm:"type:jsonb"` // Current status stored as JSON
	TargetAgentURL   string               `json:"target_agent_url"`         // URL of the agent handling this task
	TargetAgentID    string               `json:"target_agent_id"`          // Platform ID of the target agent
	PushConfig       *PushNotificationConfig `json:"push_config,omitempty" gorm:"type:jsonb"` // Optional Push config stored as JSON
	Artifacts        []Artifact           `json:"artifacts,omitempty" gorm:"type:jsonb"` // Store artifacts as JSON array
	Metadata         Metadata             `json:"metadata,omitempty" gorm:"type:jsonb"`
	CreatedAt        time.Time            `json:"created_at" gorm:"autoCreateTime"`
	LastUpdatedAt    time.Time            `json:"last_updated_at" gorm:"autoUpdateTime"`
	// Potentially add a field for the requesting user/agent ID
}

// TaskStatus represents the internal structure for task status (storable as JSON).
type TaskStatus struct {
	State     TaskState  `json:"state"`
	Message   *Message   `json:"message,omitempty"` // Stored as JSON sub-document
	Timestamp *time.Time `json:"timestamp,omitempty"`
}

// Message represents the internal structure for messages (storable as JSON).
type Message struct {
	Role     string `json:"role"` // "user" or "agent"
	Parts    []Part `json:"parts"`    // Slice of Part interfaces (see below)
	Metadata Metadata `json:"metadata,omitempty"`
}

// Artifact represents the internal structure for artifacts (storable as JSON).
type Artifact struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Parts       []Part   `json:"parts"`
	Index       int      `json:"index,omitempty"`
	Append      *bool    `json:"append,omitempty"`
	LastChunk   *bool    `json:"lastChunk,omitempty"`
	Metadata    Metadata `json:"metadata,omitempty"`
}

// Part represents a component of a Message or Artifact.
// We use specific structs here for type safety internally.
// JSON marshalling/unmarshalling needs custom handling if storing Parts directly.
// Often, the containing struct (Message, Artifact) is stored as a single JSON blob,
// relying on the `pkg/a2a` definitions during conversion.
type Part interface {
    GetType() string
}

type TextPart struct {
	Type     string   `json:"type"` // "text"
	Text     string   `json:"text"`
	Metadata Metadata `json:"metadata,omitempty"`
}
func (p TextPart) GetType() string { return p.Type }


type FilePart struct {
	Type     string      `json:"type"` // "file"
	File     FileContent `json:"file"`
	Metadata Metadata    `json:"metadata,omitempty"`
}
func (p FilePart) GetType() string { return p.Type }

type DataPart struct {
	Type     string   `json:"type"` // "data"
	Data     Metadata `json:"data"` // Use Metadata map[string]any for data
	Metadata Metadata `json:"metadata,omitempty"`
}
func (p DataPart) GetType() string { return p.Type }


// FileContent is the internal representation.
type FileContent struct {
	Name     *string `json:"name,omitempty"`
	MimeType *string `json:"mimeType,omitempty"`
	Bytes    *string `json:"bytes,omitempty"` // Base64 encoded string
	URI      *string `json:"uri,omitempty"`
}


// PushNotificationConfig internal representation.
type PushNotificationConfig struct {
	URL            string              `json:"url"`
	Token          *string             `json:"token,omitempty"`
	Authentication *AuthenticationInfo `json:"authentication,omitempty"`
}

// AuthenticationInfo internal representation.
type AuthenticationInfo struct {
	Schemes     []string `json:"schemes"`
	Credentials *string  `json:"credentials,omitempty"`
}

// AgentCard internal representation (often just the A2A struct embedded or as JSON).
// Use the a2a type directly if no internal changes are needed.
type AgentCard a2a.AgentCard // Embedding or use a2a.AgentCard directly

// Metadata is a helper type for handling JSON metadata maps.
type Metadata map[string]any

// --- JSON Marshaling/Unmarshaling for JSONB ---
// Implement Scan and Value for types stored as JSONB in the database.

// Value implements the driver.Valuer interface for Metadata.
func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal(nil) // Store as JSON null
	}
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for Metadata.
func (m *Metadata) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed for Metadata")
	}
	// Handle JSON null specifically if stored that way
	if string(bytes) == "null" {
		*m = nil
		return nil
	}
	return json.Unmarshal(bytes, m)
}


// Implement Value/Scan for other JSONB types: AgentCard, TaskStatus, Artifacts array, PushConfig etc.
// Example for TaskStatus:
func (ts TaskStatus) Value() (driver.Value, error) {
	return json.Marshal(ts)
}
func (ts *TaskStatus) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok { return errors.New("type assertion to []byte failed for TaskStatus") }
	if string(bytes) == "null" { return nil } // Handle null if necessary
	return json.Unmarshal(bytes, ts)
}

// Example for []Artifact:
type Artifacts []Artifact // Define alias for slice
func (a Artifacts) Value() (driver.Value, error) {
	if a == nil { return json.Marshal(nil) }
	return json.Marshal(a)
}
func (a *Artifacts) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok { return errors.New("type assertion to []byte failed for Artifacts") }
	if string(bytes) == "null" { *a = nil; return nil }
	return json.Unmarshal(bytes, a)
}

// Implement similarly for AgentCard, PushNotificationConfig, etc. if using JSONB


// --- Tool Definition ---
type ToolDefinition struct {
    Name        string `json:"name" gorm:"primaryKey"`
    Description string `json:"description"`
    Schema      Metadata `json:"schema" gorm:"type:jsonb"` // Input/Output schema as JSON
    // Add fields like creator, tags etc. if needed
}