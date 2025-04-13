// internal/tools/interface.go
package tools

import (
	"context"
	"github.com/kathir-ks/a2a-platform/internal/models" // Reference ToolDefinition
)

// Executor defines the interface for a runnable tool.
// Each specific tool (like a calculator or web search) will implement this.
type Executor interface {
	// GetDefinition returns the static definition of the tool (name, description, schema).
	GetDefinition() models.ToolDefinition

	// Execute runs the tool with the given parameters.
	// Parameters should conform to the input schema defined in GetDefinition().Schema.
	// The result map should conform to the output schema.
	Execute(ctx context.Context, params map[string]any) (result map[string]any, err error)
}

// Registry defines the interface for managing available tools.
// The ToolService in internal/app would likely use an implementation of this.
type Registry interface {
	// Register adds or updates a tool executor in the registry.
	Register(ctx context.Context, tool Executor) error

	// Get returns the executor for a given tool name.
	Get(ctx context.Context, toolName string) (Executor, error) // Returns error if not found

	// List returns the definitions of all registered tools.
	List(ctx context.Context) ([]models.ToolDefinition, error)
}