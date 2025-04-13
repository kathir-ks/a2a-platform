// internal/models/conversion.go
package models

import (
	"encoding/json"
	"time"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

// TaskModelToA2A converts internal Task model to A2A Task type.
func TaskModelToA2A(model *Task) *a2a.Task {
	if model == nil {
		return nil
	}
	a2aTask := &a2a.Task{
		ID:        model.ID,
		SessionID: model.SessionID,
		Status:    *TaskStatusModelToA2A(&model.Status), // Convert status
		Metadata:  model.Metadata,                     // Maps directly if types match
	}
	// Convert artifacts slice
	if model.Artifacts != nil {
		a2aTask.Artifacts = make([]a2a.Artifact, 0, len(model.Artifacts))
		for _, artModel := range model.Artifacts {
			a2aArt := ArtifactModelToA2A(&artModel)
			if a2aArt != nil {
				a2aTask.Artifacts = append(a2aTask.Artifacts, *a2aArt)
			}
		}
	}
	return a2aTask
}

// TaskA2AToModel converts A2A Task type to internal Task model.
// Note: This usually only updates parts of the model (like status, artifacts)
// rather than creating a whole new one from scratch. A Get-then-Update pattern is common.
func TaskA2AToModel(a2aTask *a2a.Task) *Task {
	if a2aTask == nil {
		return nil
	}
	model := &Task{
		ID:        a2aTask.ID,
		SessionID: a2aTask.SessionID,
		Status:    *TaskStatusA2AToModel(&a2aTask.Status), // Convert status
		Metadata:  a2aTask.Metadata,
	}
	// Convert artifacts slice
	if a2aTask.Artifacts != nil {
		model.Artifacts = make([]Artifact, 0, len(a2aTask.Artifacts))
		for _, a2aArt := range a2aTask.Artifacts {
			modelArt := ArtifactA2AToModel(&a2aArt)
			if modelArt != nil {
				model.Artifacts = append(model.Artifacts, *modelArt)
			}
		}
	}

	// Fields like TargetAgentURL, CreatedAt etc., are NOT typically set from A2A Task object
	// They are set during task creation or by the platform internally.
	return model
}


// TaskStatusModelToA2A converts internal TaskStatus to A2A TaskStatus.
func TaskStatusModelToA2A(model *TaskStatus) *a2a.TaskStatus {
	if model == nil {
		return nil
	}
	ts := &a2a.TaskStatus{
		State:   a2a.TaskState(model.State), // Cast string type
		Message: MessageModelToA2A(model.Message),
        // Timestamp handling: Internal might always have one, A2A is optional
        Timestamp: model.Timestamp, // Assign directly if types are compatible pointers
	}
    // If internal timestamp is zero time, make A2A timestamp nil
    if model.Timestamp != nil && model.Timestamp.IsZero() {
        ts.Timestamp = nil
    }
	return ts
}

// TaskStatusA2AToModel converts A2A TaskStatus to internal TaskStatus.
func TaskStatusA2AToModel(a2aStatus *a2a.TaskStatus) *TaskStatus {
	if a2aStatus == nil {
		return nil
	}
	now := time.Now().UTC() // Default timestamp if missing in A2A
	ts := a2aStatus.Timestamp
	if ts == nil || ts.IsZero() {
        ts = &now
    }

	return &TaskStatus{
		State:     TaskState(a2aStatus.State), // Cast string type
		Message:   MessageA2AToModel(a2aStatus.Message),
		Timestamp: ts,
	}
}

// MessageModelToA2A converts internal Message to A2A Message.
func MessageModelToA2A(model *Message) *a2a.Message {
	if model == nil {
		return nil
	}
	a2aMsg := &a2a.Message{
		Role:     model.Role,
		Metadata: model.Metadata,
		Parts:    make([]any, 0, len(model.Parts)), // Use []any for A2A flexibility
	}
	for _, pModel := range model.Parts {
		a2aPart := PartModelToA2A(pModel) // Convert individual part
		if a2aPart != nil {
			a2aMsg.Parts = append(a2aMsg.Parts, a2aPart)
		}
	}
	return a2aMsg
}

// MessageA2AToModel converts A2A Message to internal Message.
func MessageA2AToModel(a2aMsg *a2a.Message) *Message {
	if a2aMsg == nil {
		return nil
	}
	model := &Message{
		Role:     a2aMsg.Role,
		Metadata: a2aMsg.Metadata,
		Parts:    make([]Part, 0, len(a2aMsg.Parts)),
	}
	for _, a2aPartAny := range a2aMsg.Parts {
		modelPart := PartA2AToModel(a2aPartAny) // Convert individual part
		if modelPart != nil {
			model.Parts = append(model.Parts, modelPart)
		}
	}
	return model
}

// ArtifactModelToA2A converts internal Artifact to A2A Artifact.
func ArtifactModelToA2A(model *Artifact) *a2a.Artifact {
	if model == nil {
		return nil
	}
	a2aArt := &a2a.Artifact{
		Name:        model.Name,
		Description: model.Description,
		Index:       model.Index,
		Append:      model.Append,
		LastChunk:   model.LastChunk,
		Metadata:    model.Metadata,
		Parts:       make([]any, 0, len(model.Parts)),
	}
	for _, pModel := range model.Parts {
		a2aPart := PartModelToA2A(pModel)
		if a2aPart != nil {
			a2aArt.Parts = append(a2aArt.Parts, a2aPart)
		}
	}
	return a2aArt
}

// ArtifactA2AToModel converts A2A Artifact to internal Artifact.
func ArtifactA2AToModel(a2aArt *a2a.Artifact) *Artifact {
	if a2aArt == nil {
		return nil
	}
	model := &Artifact{
		Name:        a2aArt.Name,
		Description: a2aArt.Description,
		Index:       a2aArt.Index,
		Append:      a2aArt.Append,
		LastChunk:   a2aArt.LastChunk,
		Metadata:    a2aArt.Metadata,
		Parts:       make([]Part, 0, len(a2aArt.Parts)),
	}
	for _, a2aPartAny := range a2aArt.Parts {
		modelPart := PartA2AToModel(a2aPartAny)
		if modelPart != nil {
			model.Parts = append(model.Parts, modelPart)
		}
	}
	return model
}


// PartModelToA2A converts internal Part interface to A2A part (map[string]any).
func PartModelToA2A(model Part) any {
	if model == nil {
		return nil
	}
    // Marshal the specific model part to JSON bytes, then unmarshal to map[string]any
    // This ensures the structure matches what pkg/a2a expects for its `any` type.
	bytes, err := json.Marshal(model)
    if err != nil {
        log.Errorf("Failed to marshal model part: %v", err)
        return nil // Or handle error appropriately
    }
    var resultMap map[string]any
    err = json.Unmarshal(bytes, &resultMap)
     if err != nil {
        log.Errorf("Failed to unmarshal model part to map: %v", err)
        return nil // Or handle error appropriately
    }
	return resultMap
}

// PartA2AToModel converts A2A part (map[string]any or specific struct) to internal Part interface.
func PartA2AToModel(a2aPartAny any) Part {
	if a2aPartAny == nil {
		return nil
	}

	// 1. Marshal the incoming `any` type to bytes
	bytes, err := json.Marshal(a2aPartAny)
	if err != nil {
		log.Errorf("Failed to marshal A2A part for conversion: %v", err)
		return nil
	}

	// 2. Unmarshal just enough to get the type
	var typeFinder struct { Type string `json:"type"` }
	if err := json.Unmarshal(bytes, &typeFinder); err != nil {
		log.Errorf("Failed to determine type of A2A part: %v", err)
		return nil
	}

	// 3. Unmarshal into the correct internal model struct based on type
	switch typeFinder.Type {
	case a2a.PartTypeText:
		var part TextPart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A TextPart: %v", err)
			return nil
		}
		return part
	case a2a.PartTypeFile:
		var part FilePart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A FilePart: %v", err)
			return nil
		}
		// Convert FileContent if needed (assuming direct mapping for now)
		return part
	case a2a.PartTypeData:
		var part DataPart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A DataPart: %v", err)
			return nil
		}
		return part
	default:
		log.Warnf("Unknown A2A part type encountered: %s", typeFinder.Type)
		return nil
	}
}


// Add conversions for PushNotificationConfig, AuthenticationInfo, AgentCard etc. if needed
// Example:
func PushNotificationConfigModelToA2A(model *PushNotificationConfig) *a2a.PushNotificationConfig {
	if model == nil { return nil }
	// Assuming AuthenticationInfo maps directly or has its own converter
	return &a2a.PushNotificationConfig{
		URL: model.URL,
		Token: model.Token,
		Authentication: AuthenticationInfoModelToA2A(model.Authentication),
	}
}
func PushNotificationConfigA2AToModel(a2aConf *a2a.PushNotificationConfig) *PushNotificationConfig {
	if a2aConf == nil { return nil }
	return &PushNotificationConfig{
		URL: a2aConf.URL,
		Token: a2aConf.Token,
		Authentication: AuthenticationInfoA2AToModel(a2aConf.Authentication),
	}
}

// AuthenticationInfo converters... (assuming direct mapping for simplicity)
func AuthenticationInfoModelToA2A(model *AuthenticationInfo) *a2a.AuthenticationInfo {
	if model == nil { return nil }
	return (*a2a.AuthenticationInfo)(model) // Direct cast if fields match exactly
}
func AuthenticationInfoA2AToModel(a2aAuth *a2a.AuthenticationInfo) *AuthenticationInfo {
	if a2aAuth == nil { return nil }
	return (*AuthenticationInfo)(a2aAuth) // Direct cast if fields match exactly
}

// AgentCard converters... (if internal AgentCard struct differs from a2a.AgentCard)