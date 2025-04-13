// internal/tools/examples/calculator.go
package examples

import (
	"context"
	"fmt"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/tools"
)

// CalculatorTool provides basic arithmetic operations.
type CalculatorTool struct{}

var _ tools.Executor = (*CalculatorTool)(nil) // Compile-time check for interface implementation

// GetDefinition implements the tools.Executor interface.
func (t *CalculatorTool) GetDefinition() models.ToolDefinition {
	return models.ToolDefinition{
		Name:        "calculator",
		Description: "Performs basic arithmetic operations: add, subtract, multiply, divide.",
		Schema: models.Metadata{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type": "string",
					"enum": []string{"add", "subtract", "multiply", "divide"},
                    "description": "The arithmetic operation to perform.",
				},
				"operand1": map[string]any{
					"type": "number",
                    "description": "The first number.",
				},
				"operand2": map[string]any{
					"type": "number",
                    "description": "The second number.",
				},
			},
			"required": []string{"operation", "operand1", "operand2"},
            // Output schema
            "output": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "result": map[string]any{
                        "type": "number",
                        "description": "The result of the calculation.",
                    },
                },
                 "required": []string{"result"},
            },
		},
	}
}

// Execute implements the tools.Executor interface.
func (t *CalculatorTool) Execute(ctx context.Context, params map[string]any) (result map[string]any, err error) {
	// Basic type assertion and validation (more robust validation using schema is better)
	op, ok := params["operation"].(string)
	if !ok { return nil, fmt.Errorf("invalid or missing 'operation' parameter (string expected)") }

	op1, ok := params["operand1"].(float64) // JSON numbers are typically float64
	if !ok { return nil, fmt.Errorf("invalid or missing 'operand1' parameter (number expected)") }

	op2, ok := params["operand2"].(float64)
	if !ok { return nil, fmt.Errorf("invalid or missing 'operand2' parameter (number expected)") }

	var calculationResult float64

	switch op {
	case "add":
		calculationResult = op1 + op2
	case "subtract":
		calculationResult = op1 - op2
	case "multiply":
		calculationResult = op1 * op2
	case "divide":
		if op2 == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		calculationResult = op1 / op2
	default:
		return nil, fmt.Errorf("unknown operation: %s", op)
	}

	// Return result conforming to the output schema
	result = map[string]any{
		"result": calculationResult,
	}
	return result, nil
}