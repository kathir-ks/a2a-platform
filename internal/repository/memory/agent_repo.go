// internal/repository/memory/agent_repo.go
package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
    "time"

	"github.com/kathir-ks/a2a-platform/internal/models"
	"github.com/kathir-ks/a2a-platform/internal/repository"
	log "github.com/sirupsen/logrus"
)

type memoryAgentRepository struct {
	mu      sync.RWMutex
	agents  map[string]*models.Agent // agentID -> Agent
	urlMap  map[string]string      // agent URL -> agentID (for FindByURL)
}

// NewMemoryAgentRepository creates a new in-memory agent repository.
func NewMemoryAgentRepository() repository.AgentRepository {
	return &memoryAgentRepository{
		agents: make(map[string]*models.Agent),
		urlMap: make(map[string]string),
	}
}

// deepCopyAgent creates a deep copy.
func deepCopyAgent(original *models.Agent) (*models.Agent, error) {
	if original == nil {
		return nil, nil
	}
	cpy := &models.Agent{}
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

func (r *memoryAgentRepository) Create(ctx context.Context, agent *models.Agent) (*models.Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agent.ID]; exists {
		return nil, ErrAlreadyExists
	}
    // Check URL uniqueness if required by the platform
    if agent.AgentCard.URL != "" {
         if _, urlExists := r.urlMap[agent.AgentCard.URL]; urlExists {
             return nil, ErrAlreadyExists // Or a more specific URL conflict error
         }
    }

	agentCopy, err := deepCopyAgent(agent)
	if err != nil {
		log.Errorf("MemoryRepo: Failed to deep copy agent on create %s: %v", agent.ID, err)
		return nil, err
	}
     if agentCopy.RegisteredAt.IsZero() {
        agentCopy.RegisteredAt = time.Now().UTC()
    }
    agentCopy.LastUpdatedAt = agentCopy.RegisteredAt
    // Default IsEnabled? Let's default to true
    agentCopy.IsEnabled = true

	r.agents[agent.ID] = agentCopy
    if agentCopy.AgentCard.URL != "" {
	    r.urlMap[agentCopy.AgentCard.URL] = agent.ID
    }
	log.Debugf("MemoryRepo: Created agent %s (URL: %s)", agent.ID, agent.AgentCard.URL)

	return deepCopyAgent(agentCopy)
}

func (r *memoryAgentRepository) Update(ctx context.Context, agent *models.Agent) (*models.Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existingAgent, exists := r.agents[agent.ID]
	if !exists {
		return nil, ErrNotFound
	}

    // Check for URL change and potential conflict
    newURL := agent.AgentCard.URL
    oldURL := existingAgent.AgentCard.URL
    if newURL != oldURL {
        // Check if new URL conflicts with another agent
        if existingAgentID, urlExists := r.urlMap[newURL]; urlExists && existingAgentID != agent.ID {
             return nil, ErrAlreadyExists // Or a URL conflict error
        }
    }

	agentCopy, err := deepCopyAgent(agent)
	if err != nil {
		log.Errorf("MemoryRepo: Failed to deep copy agent on update %s: %v", agent.ID, err)
		return nil, err
	}
    // Preserve registration time, update other fields
    agentCopy.RegisteredAt = existingAgent.RegisteredAt
    agentCopy.LastUpdatedAt = time.Now().UTC()

	r.agents[agent.ID] = agentCopy

    // Update URL map if necessary
    if newURL != oldURL {
        if oldURL != "" {
            delete(r.urlMap, oldURL) // Remove old mapping
        }
        if newURL != "" {
            r.urlMap[newURL] = agent.ID // Add new mapping
        }
    }

	log.Debugf("MemoryRepo: Updated agent %s (URL: %s)", agent.ID, agent.AgentCard.URL)
	return deepCopyAgent(agentCopy)
}

func (r *memoryAgentRepository) FindByID(ctx context.Context, id string) (*models.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[id]
	if !exists {
		return nil, ErrNotFound
	}
	log.Debugf("MemoryRepo: Found agent %s by ID", id)
	return deepCopyAgent(agent)
}

func (r *memoryAgentRepository) FindByURL(ctx context.Context, url string) (*models.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agentID, urlMapped := r.urlMap[url]
	if !urlMapped {
		return nil, ErrNotFound
	}

	agent, exists := r.agents[agentID]
	if !exists {
        // Data inconsistency! URL map points to non-existent agent. Log error.
        log.Errorf("MemoryRepo: Data inconsistency - URL map points to missing agent ID %s for URL %s", agentID, url)
		return nil, ErrNotFound // Or internal error? ErrNotFound is safer for caller.
	}
	log.Debugf("MemoryRepo: Found agent %s by URL %s", agentID, url)
	return deepCopyAgent(agent)
}

func (r *memoryAgentRepository) List(ctx context.Context, limit, offset int) ([]*models.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Extract agents into a slice for sorting/pagination
	agentList := make([]*models.Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agentList = append(agentList, agent)
	}

	// Sort for consistent pagination (e.g., by ID)
	sort.Slice(agentList, func(i, j int) bool {
		return agentList[i].ID < agentList[j].ID
	})

	// Apply pagination
	start := offset
	if start < 0 {
		start = 0
	}
	if start >= len(agentList) {
		return []*models.Agent{}, nil // Offset out of bounds
	}

	end := start + limit
	if end > len(agentList) || limit <= 0 { // Handle limit <= 0 as "no limit"
		end = len(agentList)
	}

    paginatedList := agentList[start:end]

    // Create deep copies of the result slice
    results := make([]*models.Agent, 0, len(paginatedList))
    for _, agent := range paginatedList {
        agentCopy, err := deepCopyAgent(agent)
        if err != nil {
            log.Errorf("MemoryRepo: Failed to deep copy agent %s during List: %v", agent.ID, err)
            return nil, err // Propagate copy error
        }
        results = append(results, agentCopy)
    }

	log.Debugf("MemoryRepo: Listed agents (Limit: %d, Offset: %d) - returning %d", limit, offset, len(results))
	return results, nil
}