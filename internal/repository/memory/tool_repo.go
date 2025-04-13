// internal/repository/memory/tool_repo.go
package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	log "github.com/sirupsen/logrus"
)

type memoryToolRepository struct {
	mu    sync.RWMutex
	tools map[string]*models.ToolDefinition // tool name -> ToolDefinition
}

// NewMemoryToolRepository creates a new in-memory tool repository.
func NewMemoryToolRepository() repository.ToolRepository {
	return &memoryToolRepository{
		tools: make(map[string]*models.ToolDefinition),
	}
}

// deepCopyTool creates a deep copy.
func deepCopyTool(original *models.ToolDefinition) (*models.ToolDefinition, error) {
	if original == nil {
		return nil, nil
	}
	cpy := &models.ToolDefinition{}
	bytes, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, cpy)
	if err != nil {
		return nil, err
	}
	return cpy, nil
}


func (r *memoryToolRepository) Create(ctx context.Context, tool *models.ToolDefinition) (*models.ToolDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name]; exists {
		return nil, ErrAlreadyExists
	}

	toolCopy, err := deepCopyTool(tool)
	if err != nil {
		log.Errorf("MemoryRepo: Failed to deep copy tool on create %s: %v", tool.Name, err)
		return nil, err
	}

	r.tools[tool.Name] = toolCopy
	log.Debugf("MemoryRepo: Created tool %s", tool.Name)

	return deepCopyTool(toolCopy)
}

func (r *memoryToolRepository) FindByName(ctx context.Context, name string) (*models.ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, ErrNotFound
	}
	log.Debugf("MemoryRepo: Found tool %s", name)
	return deepCopyTool(tool)
}

func (r *memoryToolRepository) List(ctx context.Context, limit, offset int) ([]*models.ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	toolList := make([]*models.ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		toolList = append(toolList, tool)
	}

	// Sort by name for consistent pagination
	sort.Slice(toolList, func(i, j int) bool {
		return toolList[i].Name < toolList[j].Name
	})

	// Apply pagination
	start := offset
	if start < 0 {
		start = 0
	}
	if start >= len(toolList) {
		return []*models.ToolDefinition{}, nil
	}

	end := start + limit
	if end > len(toolList) || limit <= 0 {
		end = len(toolList)
	}

	paginatedList := toolList[start:end]

    // Create deep copies
    results := make([]*models.ToolDefinition, 0, len(paginatedList))
     for _, tool := range paginatedList {
        toolCopy, err := deepCopyTool(tool)
        if err != nil {
            log.Errorf("MemoryRepo: Failed to deep copy tool %s during List: %v", tool.Name, err)
            return nil, err
        }
        results = append(results, toolCopy)
    }


	log.Debugf("MemoryRepo: Listed tools (Limit: %d, Offset: %d) - returning %d", limit, offset, len(results))
	return results, nil
}