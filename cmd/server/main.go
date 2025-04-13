// cmd/server/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kathir-ks/a2a-platform/internal/agentruntime"
	"github.com/kathir-ks/a2a-platform/internal/api"
	"github.com/kathir-ks/a2a-platform/internal/app"
	"github.com/kathir-ks/a2a-platform/internal/config"
	"github.com/kathir-ks/a2a-platform/internal/llmclient" // Import LLM client
	"github.com/kathir-ks/a2a-platform/internal/repository" // Import repository interfaces
	"github.com/kathir-ks/a2a-platform/internal/repository/memory" // Import memory repo implementation
	// "github.com/kathir-ks/a2a-platform/internal/repository/sql" // Or SQL implementation
	"github.com/kathir-ks/a2a-platform/internal/tools" // Import tools package
	"github.com/kathir-ks/a2a-platform/internal/ws"    // Import WS manager

	log "github.com/sirupsen/logrus"
)

func main() {
	// --- Configuration ---
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// --- Logging ---
	setupLogging(cfg.LogLevel)
	log.Info("Starting A2A Platform Backend...")

	// --- Database & Repositories ---
	// Choose repository implementation (Memory example)
	taskRepo := memory.NewMemoryTaskRepository()
	agentRepo := memory.NewMemoryAgentRepository()
	toolRepo := memory.NewMemoryToolRepository() // Assuming memory repo for tools exists
	// db := setupDatabase(cfg.DatabaseURL) // For SQL
	// taskRepo := sql.NewSQLTaskRepository(db) // For SQL

	// --- LLM Clients ---
	llmClients := initializeLLMClients(cfg) // Initialize LLM clients based on config

	// --- Agent Runtime Client ---
	agentRtClient := agentruntime.NewHTTPClient(cfg)

	// --- Tools ---
	toolRegistry := tools.NewMemoryRegistry()
	// Initialize and register the LLM Tool
	if len(llmClients) > 0 {
		llmTool, err := tools.NewLLMTool(llmClients, cfg.DefaultLLMModel)
		if err != nil {
			log.Warnf("Failed to initialize LLM Tool: %v. LLM tool will be unavailable.", err)
		} else {
			if err := toolRegistry.Register(context.Background(), llmTool); err != nil { // Use background context for initial registration
				log.Errorf("Failed to register LLM tool: %v", err)
			}
			// Register other tools (e.g., Calculator)
			if err := toolRegistry.Register(context.Background(), &tools.CalculatorTool{}); err != nil {
                 log.Errorf("Failed to register Calculator tool: %v", err)
            }
		}
	} else {
		log.Info("No LLM providers configured, LLM tool will not be available.")
	}


	// --- Application Services ---
	// Create dependency structs
	taskServiceDeps := app.TaskServiceDeps{TaskRepo: taskRepo}
	platformServiceDeps := app.PlatformServiceDeps{
		TaskSvc:    nil, // TaskService needs to be created first
		AgentRtCli: agentRtClient,
	}
    toolServiceDeps := app.ToolServiceDeps{ // Assuming ToolServiceDeps struct exists
        ToolRepo: toolRepo, // DB storage for tool defs (optional)
        Registry: toolRegistry, // Runtime execution registry
    }

	// Create services, injecting dependencies
	taskService := app.NewTaskService(taskServiceDeps)
	platformServiceDeps.TaskSvc = taskService // Now inject task service
	platformService := app.NewPlatformService(platformServiceDeps)
    toolService := app.NewToolService(toolServiceDeps) // Assuming NewToolService exists

	// --- WebSocket Manager ---
	wsAppServices := &ws.AppServices{ // Pass needed services to WS Manager
		PlatformService: platformService,
		TaskService:     taskService,
	}
	wsManager := ws.NewConnectionManager(wsAppServices)

	// --- API Router ---
	router := api.NewRouter(platformService, taskService, wsManager) // Pass services and WS manager

	// --- HTTP Server ---
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Graceful Shutdown ---
	go func() {
		log.Infof("Server starting on port %d", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v\n", server.Addr, err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Server shutting down...")

	// Create context for shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased timeout
	defer cancel()

	// Shutdown WebSocket Manager first
	wsManager.Close()

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	// Close database connection if applicable
	// closeDatabase(db)

	log.Info("Server gracefully stopped")
}

// setupLogging configures the logger.
func setupLogging(logLevel string) {
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Warnf("Invalid log level '%s', defaulting to 'info'. Error: %v", logLevel, err)
		level = log.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
        TimestampFormat: time.RFC3339,
	})
	// Or use JSONFormatter: log.SetFormatter(&log.JSONFormatter{})
}

// initializeLLMClients creates LLM client instances based on config.
func initializeLLMClients(cfg *config.Config) []llmclient.Client {
	clients := make([]llmclient.Client, 0, len(cfg.LLMProviders))

	for providerName, providerCfg := range cfg.LLMProviders {
		if providerCfg.APIKey == "" {
			log.Warnf("API key missing for LLM provider '%s', skipping client initialization.", providerName)
			continue
		}

		var client llmclient.Client
		var err error

		switch strings.ToLower(providerName) {
		case "openai":
			client, err = llmclient.NewOpenAIClient(providerCfg.APIKey)
			if err != nil {
				log.Errorf("Failed to create OpenAI client: %v", err)
			}
		case "anthropic":
			// client, err = llmclient.NewAnthropicClient(providerCfg.APIKey)
			log.Warn("Anthropic client not implemented yet")
			// if err != nil { log.Errorf(...) }
		// Add cases for other providers (google, cohere, etc.)
		default:
			log.Warnf("Unsupported LLM provider configured: %s", providerName)
		}

		if client != nil && err == nil {
			clients = append(clients, client)
		}
	}
	return clients
}


// --- Placeholder database functions (replace with real implementation) ---
// func setupDatabase(dbURL string) *sql.DB { // Or *gorm.DB
//    log.Warnf("Database setup not fully implemented. Using placeholder.")
//     // Connect to DB based on URL (Postgres, SQLite)
//     // Run migrations
//     return nil
// }
// func closeDatabase(db *sql.DB) { // Or *gorm.DB
//     if db != nil {
//          log.Info("Closing database connection...")
//          // db.Close()
//      }
// }

// --- Placeholder for Memory Tool Repo ---
package memory

import (
    "context"
    "fmt"
    "sync"
    "github.com/kathir-ks/a2a-platform/internal/models"
    "github.com/kathir-ks/a2a-platform/internal/repository"
)
type memoryToolRepository struct {
	mu    sync.RWMutex
	tools map[string]*models.ToolDefinition
}
func NewMemoryToolRepository() repository.ToolRepository {
	return &memoryToolRepository{tools: make(map[string]*models.ToolDefinition)}
}
func (r *memoryToolRepository) Create(ctx context.Context, tool *models.ToolDefinition) (*models.ToolDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Name]; exists { return nil, fmt.Errorf("tool %s already exists", tool.Name)}
	r.tools[tool.Name] = tool
	return tool, nil
}
func (r *memoryToolRepository) FindByName(ctx context.Context, name string) (*models.ToolDefinition, error) {
	r.mu.RLock(); defer r.mu.RUnlock()
	tool, ok := r.tools[name]; if !ok { return nil, fmt.Errorf("tool %s not found", name)} // Simulate ErrNoRows
	return tool, nil
}
func (r *memoryToolRepository) List(ctx context.Context, limit, offset int) ([]*models.ToolDefinition, error) {
	r.mu.RLock(); defer r.mu.RUnlock()
    list := make([]*models.ToolDefinition, 0, len(r.tools))
    for _, t := range r.tools { list = append(list, t) }
    // Apply basic limit/offset (sorting would require more)
    start := offset; if start >= len(list) { return []*models.ToolDefinition{}, nil }
    end := start + limit; if end > len(list) { end = len(list) }
    return list[start:end], nil
}

// --- Placeholder for Tool Service ---
package app
import (
    "context"
    "fmt"
	"github.com/kathir-ks/a2a-platform/internal/models"
    "github.com/kathir-ks/a2a-platform/internal/tools"
    "github.com/kathir-ks/a2a-platform/internal/repository"
)
type ToolServiceDeps struct {
    ToolRepo repository.ToolRepository
    Registry tools.Registry
}
type toolService struct {
    repo repository.ToolRepository
    registry tools.Registry
}
func NewToolService(deps ToolServiceDeps) ToolService { // Implements app.ToolService
	if deps.Registry == nil { panic("ToolService requires a non-nil Registry") }
    // Repo is optional for now if only using runtime registry
	return &toolService{repo: deps.ToolRepo, registry: deps.Registry}
}
func (s *toolService) RegisterTool(ctx context.Context, tool models.ToolDefinition) error {
    // Logic to register in DB (s.repo) if needed
	return fmt.Errorf("dynamic tool registration not implemented via service yet")
}
func (s *toolService) GetToolSchema(ctx context.Context, toolName string) (any, error) {
    executor, err := s.registry.Get(ctx, toolName)
    if err != nil { return nil, err }
    return executor.GetDefinition().Schema, nil
}
func (s *toolService) ExecuteTool(ctx context.Context, toolName string, params map[string]any) (map[string]any, error) {
     executor, err := s.registry.Get(ctx, toolName)
    if err != nil { return nil, err }
    return executor.Execute(ctx, params)
}
func (s *toolService) ListTools(ctx context.Context) ([]models.ToolDefinition, error) {
     return s.registry.List(ctx)
     // Or fetch from s.repo if definitions are persisted separately
}