// internal/models/conversion.go
package models

import (
	"encoding/json"
	"fmt" // <-- Import fmt package
	"time"

	"github.com/kathir-ks/a2a-platform/pkg/a2a"
	log "github.com/sirupsen/logrus"
)

// TaskModelToA2A converts internal Task model to A2A Task type.
// Returns the A2A Task struct value and an error if conversion fails.
func TaskModelToA2A(model *Task) (a2a.Task, error) {
	if model == nil {
		return a2a.Task{}, fmt.Errorf("cannot convert nil models.Task")
	}

	a2aStatus, err := TaskStatusModelToA2A(&model.Status) // Convert status
	if err != nil {
		return a2a.Task{}, fmt.Errorf("failed to convert task status for task %s: %w", model.ID, err)
	}

	a2aTask := a2a.Task{
		ID:        model.ID,
		SessionID: model.SessionID,
		Status:    a2aStatus, // Assign the converted struct value
		Metadata:  model.Metadata,
	}

	// Convert artifacts slice
	if model.Artifacts != nil {
		a2aTask.Artifacts = make([]a2a.Artifact, 0, len(model.Artifacts))
		for _, artModel := range model.Artifacts {
			// Assume ArtifactModelToA2A now returns (a2a.Artifact, error)
			a2aArt, artErr := ArtifactModelToA2A(&artModel)
			if artErr != nil {
				// Log error but potentially continue? Or fail entire conversion?
				log.Warnf("Failed to convert artifact during task %s conversion: %v", model.ID, artErr)
				// Let's skip the failed artifact for now
				continue
				// return a2a.Task{}, fmt.Errorf("failed to convert artifact for task %s: %w", model.ID, artErr) // Option to fail hard
			}
			a2aTask.Artifacts = append(a2aTask.Artifacts, a2aArt)
		}
	}
	return a2aTask, nil
}

// TaskA2AToModel converts A2A Task type to internal Task model.
// Note: This usually only updates parts of the model (like status, artifacts).
// It's less likely to return an error unless the input is fundamentally malformed.
func TaskA2AToModel(a2aTask *a2a.Task) (*Task, error) {
	if a2aTask == nil {
		return nil, fmt.Errorf("cannot convert nil a2a.Task")
	}

	// Assume TaskStatusA2AToModel now returns (*TaskStatus, error)
	modelStatus, statusErr := TaskStatusA2AToModel(&a2aTask.Status)
	if statusErr != nil {
		return nil, fmt.Errorf("failed to convert task status from a2a: %w", statusErr)
	}
	if modelStatus == nil { // Handle case where converter returns nil pointer on non-fatal issue
		return nil, fmt.Errorf("task status conversion resulted in nil")
	}


	model := &Task{
		ID:        a2aTask.ID,
		SessionID: a2aTask.SessionID,
		Status:    *modelStatus, // Dereference the pointer
		Metadata:  a2aTask.Metadata,
	}

	// Convert artifacts slice
	if a2aTask.Artifacts != nil {
		model.Artifacts = make([]Artifact, 0, len(a2aTask.Artifacts))
		for _, a2aArt := range a2aTask.Artifacts {
			// Assume ArtifactA2AToModel returns (*Artifact, error)
			modelArt, artErr := ArtifactA2AToModel(&a2aArt)
			if artErr != nil {
				log.Warnf("Failed to convert artifact from a2a during task %s conversion: %v", a2aTask.ID, artErr)
				continue // Skip bad artifact
				// return nil, fmt.Errorf("failed to convert artifact from a2a: %w", artErr) // Fail hard
			}
			if modelArt != nil { // Check if conversion was successful
				model.Artifacts = append(model.Artifacts, *modelArt)
			}
		}
	}

	// Fields like TargetAgentURL, CreatedAt etc., are NOT typically set from A2A Task object
	return model, nil
}


// TaskStatusModelToA2A converts internal TaskStatus to A2A TaskStatus.
// Returns the struct value and error.
func TaskStatusModelToA2A(model *TaskStatus) (a2a.TaskStatus, error) {
	if model == nil {
		return a2a.TaskStatus{}, fmt.Errorf("cannot convert nil models.TaskStatus")
	}

	// Assume MessageModelToA2A returns (*a2a.Message, error)
	a2aMessage, msgErr := MessageModelToA2A(model.Message)
	if msgErr != nil {
		return a2a.TaskStatus{}, fmt.Errorf("failed to convert message in task status: %w", msgErr)
	}

	ts := a2a.TaskStatus{
		State:     a2a.TaskState(model.State), // Cast string type
		Message:   a2aMessage, // Assign pointer directly
		Timestamp: model.Timestamp,
	}
	if model.Timestamp != nil && model.Timestamp.IsZero() {
		ts.Timestamp = nil
	}
	return ts, nil
}

// TaskStatusA2AToModel converts A2A TaskStatus to internal TaskStatus pointer and error.
func TaskStatusA2AToModel(a2aStatus *a2a.TaskStatus) (*TaskStatus, error) {
	if a2aStatus == nil {
		return nil, fmt.Errorf("cannot convert nil a2a.TaskStatus")
	}

	now := time.Now().UTC()
	ts := a2aStatus.Timestamp
	if ts == nil || ts.IsZero() {
		ts = &now
	}

	// Assume MessageA2AToModel returns (*Message, error)
	modelMessage, msgErr := MessageA2AToModel(a2aStatus.Message)
	if msgErr != nil {
		return nil, fmt.Errorf("failed to convert message from a2a task status: %w", msgErr)
	}

	return &TaskStatus{
		State:     TaskState(a2aStatus.State),
		Message:   modelMessage, // Assign pointer directly
		Timestamp: ts,
	}, nil
}

// MessageModelToA2A converts internal Message to A2A Message pointer and error.
func MessageModelToA2A(model *Message) (*a2a.Message, error) {
	if model == nil {
		return nil, nil // It's okay for a message to be nil, not an error
	}
	a2aMsg := &a2a.Message{
		Role:     model.Role,
		Metadata: model.Metadata,
		Parts:    make([]any, 0, len(model.Parts)),
	}
	for _, pModel := range model.Parts {
		// Assume PartModelToA2A returns (any, error)
		a2aPart, partErr := PartModelToA2A(pModel)
		if partErr != nil {
			return nil, fmt.Errorf("failed to convert part in message: %w", partErr)
		}
		if a2aPart != nil {
			a2aMsg.Parts = append(a2aMsg.Parts, a2aPart)
		}
	}
	return a2aMsg, nil
}

// MessageA2AToModel converts A2A Message to internal Message pointer and error.
func MessageA2AToModel(a2aMsg *a2a.Message) (*Message, error) {
	if a2aMsg == nil {
		return nil, nil // Okay for message to be nil
	}
	model := &Message{
		Role:     a2aMsg.Role,
		Metadata: a2aMsg.Metadata,
		Parts:    make([]Part, 0, len(a2aMsg.Parts)),
	}
	for _, a2aPartAny := range a2aMsg.Parts {
		// Assume PartA2AToModel returns (Part, error)
		modelPart, partErr := PartA2AToModel(a2aPartAny)
		if partErr != nil {
			return nil, fmt.Errorf("failed to convert part from a2a message: %w", partErr)
		}
		if modelPart != nil {
			model.Parts = append(model.Parts, modelPart)
		}
	}
	return model, nil
}

// ArtifactModelToA2A converts internal Artifact to A2A Artifact value and error.
func ArtifactModelToA2A(model *Artifact) (a2a.Artifact, error) {
	if model == nil {
		return a2a.Artifact{}, fmt.Errorf("cannot convert nil models.Artifact")
	}
	a2aArt := a2a.Artifact{
		Name:        model.Name,
		Description: model.Description,
		Index:       model.Index,
		Append:      model.Append,
		LastChunk:   model.LastChunk,
		Metadata:    model.Metadata,
		Parts:       make([]any, 0, len(model.Parts)),
	}
	for _, pModel := range model.Parts {
		a2aPart, partErr := PartModelToA2A(pModel)
		if partErr != nil {
			return a2a.Artifact{}, fmt.Errorf("failed to convert part in artifact: %w", partErr)
		}
		if a2aPart != nil {
			a2aArt.Parts = append(a2aArt.Parts, a2aPart)
		}
	}
	return a2aArt, nil
}

// ArtifactA2AToModel converts A2A Artifact to internal Artifact pointer and error.
func ArtifactA2AToModel(a2aArt *a2a.Artifact) (*Artifact, error) {
	if a2aArt == nil {
		return nil, fmt.Errorf("cannot convert nil a2a.Artifact")
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
		modelPart, partErr := PartA2AToModel(a2aPartAny)
		if partErr != nil {
			return nil, fmt.Errorf("failed to convert part from a2a artifact: %w", partErr)
		}
		if modelPart != nil {
			model.Parts = append(model.Parts, modelPart)
		}
	}
	return model, nil
}


// PartModelToA2A converts internal Part interface to A2A part (map[string]any) and error.
func PartModelToA2A(model Part) (any, error) {
	if model == nil {
		return nil, nil // Nil part is okay
	}
	bytes, err := json.Marshal(model)
	if err != nil {
		log.Errorf("Failed to marshal model part: %v", err)
		return nil, fmt.Errorf("failed to marshal model part: %w", err)
	}
	var resultMap map[string]any
	err = json.Unmarshal(bytes, &resultMap)
	if err != nil {
		log.Errorf("Failed to unmarshal model part to map: %v", err)
		return nil, fmt.Errorf("failed to unmarshal model part to map: %w", err)
	}
	return resultMap, nil
}

// PartA2AToModel converts A2A part (map[string]any or specific struct) to internal Part interface and error.
func PartA2AToModel(a2aPartAny any) (Part, error) {
	if a2aPartAny == nil {
		return nil, nil // Nil part is okay
	}

	bytes, err := json.Marshal(a2aPartAny)
	if err != nil {
		log.Errorf("Failed to marshal A2A part for conversion: %v", err)
		return nil, fmt.Errorf("failed to marshal A2A part: %w", err)
	}

	var typeFinder struct { Type string `json:"type"` }
	if err := json.Unmarshal(bytes, &typeFinder); err != nil {
		log.Errorf("Failed to determine type of A2A part: %v", err)
		// Try decoding as just text if type is missing? Risky.
		return nil, fmt.Errorf("cannot determine type of A2A part: %w", err)
	}

	switch typeFinder.Type {
	case a2a.PartTypeText:
		var part TextPart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A TextPart: %v", err)
			return nil, fmt.Errorf("failed to decode A2A TextPart: %w", err)
		}
		return part, nil
	case a2a.PartTypeFile:
		var part FilePart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A FilePart: %v", err)
			return nil, fmt.Errorf("failed to decode A2A FilePart: %w", err)
		}
		// Add FileContent conversion logic if needed
		return part, nil
	case a2a.PartTypeData:
		var part DataPart
		if err := json.Unmarshal(bytes, &part); err != nil {
			log.Errorf("Failed to unmarshal A2A DataPart: %v", err)
			return nil, fmt.Errorf("failed to decode A2A DataPart: %w", err)
		}
		return part, nil
	default:
		log.Warnf("Unknown A2A part type encountered: %s", typeFinder.Type)
		return nil, fmt.Errorf("unknown A2A part type '%s'", typeFinder.Type)
	}
}


// --- Other Converters (simplified, add error returns if needed) ---

func PushNotificationConfigModelToA2A(model *PushNotificationConfig) (*a2a.PushNotificationConfig, error) {
	if model == nil { return nil, nil }
	authA2A, err := AuthenticationInfoModelToA2A(model.Authentication)
	if err != nil {
		return nil, fmt.Errorf("failed converting auth info for push config: %w", err)
	}
	return &a2a.PushNotificationConfig{
		URL:            model.URL,
		Token:          model.Token,
		Authentication: authA2A,
	}, nil
}

func PushNotificationConfigA2AToModel(a2aConf *a2a.PushNotificationConfig) (*PushNotificationConfig, error) {
	if a2aConf == nil { return nil, nil }
	authModel, err := AuthenticationInfoA2AToModel(a2aConf.Authentication)
	if err != nil {
		return nil, fmt.Errorf("failed converting auth info from a2a push config: %w", err)
	}
	return &PushNotificationConfig{
		URL:            a2aConf.URL,
		Token:          a2aConf.Token,
		Authentication: authModel,
	}, nil
}

func AuthenticationInfoModelToA2A(model *AuthenticationInfo) (*a2a.AuthenticationInfo, error) {
	if model == nil { return nil, nil }
	// Direct mapping assumed safe here, add validation/error if needed
	return &a2a.AuthenticationInfo{
		Schemes:     model.Schemes,
		Credentials: model.Credentials,
	}, nil
}

func AuthenticationInfoA2AToModel(a2aAuth *a2a.AuthenticationInfo) (*AuthenticationInfo, error) {
	if a2aAuth == nil { return nil, nil }
	// Direct mapping assumed safe here, add validation/error if needed
	return &AuthenticationInfo{
		Schemes:     a2aAuth.Schemes,
		Credentials: a2aAuth.Credentials,
	}, nil
}