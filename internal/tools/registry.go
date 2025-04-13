// internal/tools/registry.go
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/kathir-ks/a2a-platform/internal/models"
	log "github.com/sirupsen/logrus"
)

// memoryRegistry provides an in-memory implementation of the tools.Registry interface.
type memoryRegistry struct {
	mu    sync.RWMutex
	tools map[string]Executor // Map tool name to its executor
}

// NewMemoryRegistry creates a new in-memory tool registry.
func NewMemoryRegistry() Registry {
	return &memoryRegistry{
		tools: make(map[string]Executor),
	}
}

// Register implements the Registry interface.
func (r *memoryRegistry) Register(ctx context.Context, tool Executor) error {
	if tool == nil {
		return fmt.Errorf("cannot register a nil tool")
	}
	def := tool.GetDefinition()
	if def.Name == "" {
		return fmt.Errorf("cannot register tool with empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[def.Name]; exists {
		log.Warnf("Tool '%s' is being replaced in the registry", def.Name)
	} else {
		log.Infof("Registering tool: %s", def.Name)
	}
	r.tools[def.Name] = tool

	return nil
}

// Get implements the Registry interface.
func (r *memoryRegistry) Get(ctx context.Context, toolName string) (Executor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found", toolName) // Specific error
	}
	return tool, nil
}

// List implements the Registry interface.
func (r *memoryRegistry) List(ctx context.Context) ([]models.ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]models.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		defs = append(defs, tool.GetDefinition())
	}
	return defs, nil
}