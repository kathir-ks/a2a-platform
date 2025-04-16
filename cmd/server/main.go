// cmd/server/main.go
package main

import (
	"context"
	"errors" // Required for checking ErrAlreadyExists
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings" // Needed for initializeLLMClients
	"syscall"
	"time"

	"github.com/kathir-ks/a2a-platform/internal/agentruntime"
	"github.com/kathir-ks/a2a-platform/internal/api"
	"github.com/kathir-ks/a2a-platform/internal/app"
	"github.com/kathir-ks/a2a-platform/internal/config"
	"github.com/kathir-ks/a2a-platform/internal/llmclient"
	memRepo "github.com/kathir-ks/a2a-platform/internal/repository/memory" // Alias memory repo
	// Import repository interfaces IF needed directly, usually not needed here
	// "github.com/kathir-ks/a2a-platform/internal/repository"
	// "github.com/kathir-ks/a2a-platform/internal/repository/sql" // Or SQL implementation
	"github.com/kathir-ks/a2a-platform/internal/tools"
	"github.com/kathir-ks/a2a-platform/internal/tools/examples" // Import for CalculatorTool struct
	"github.com/kathir-ks/a2a-platform/internal/ws"             // Import WS manager
	"github.com/kathir-ks/a2a-platform/pkg/a2a"                 // Import A2A types for example agents

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
	log.Info("Using IN-MEMORY repositories (Data will be lost on shutdown)")
	taskRepo := memRepo.NewMemoryTaskRepository()
	agentRepo := memRepo.NewMemoryAgentRepository()
	toolRepo := memRepo.NewMemoryToolRepository()
	// db := setupDatabase(cfg.DatabaseURL) // For SQL
	// taskRepo := sql.NewSQLTaskRepository(db) // For SQL
	// agentRepo := sql.NewMemoryAgentRepository(db) // For SQL
	// toolRepo := sql.NewMemoryToolRepository(db) // For SQL

	// --- LLM Clients ---
	llmClients := initializeLLMClients(cfg) // Initialize LLM clients based on config

	// --- Agent Runtime Client ---
	agentRtClient := agentruntime.NewHTTPClient(cfg)

	// --- Tools Registry & Tools ---
	toolRegistry := tools.NewMemoryRegistry()
	// Initialize and register the LLM Tool
	if len(llmClients) > 0 {
		llmTool, err := tools.NewLLMTool(llmClients, cfg.DefaultLLMModel)
		if err != nil {
			log.Warnf("Failed to initialize LLM Tool: %v. LLM tool will be unavailable.", err)
		} else {
			// Use background context for initial registrations
			bgCtx := context.Background()
			if err := toolRegistry.Register(bgCtx, llmTool); err != nil {
				log.Errorf("Failed to register LLM tool: %v", err)
			}
		}
	} else {
		log.Info("No LLM providers configured, LLM tool will not be available.")
	}
	// Register other tools (e.g., Calculator)
	if err := toolRegistry.Register(context.Background(), &examples.CalculatorTool{}); err != nil {
		log.Errorf("Failed to register Calculator tool: %v", err)
	}
	// Register example web search tool (if it exists - placeholder)
	// if err := toolRegistry.Register(context.Background(), &examples.WebSearchTool{}); err != nil {
	// 	log.Errorf("Failed to register WebSearch tool: %v", err)
	// }

	// --- Application Services ---
	// Create dependency structs (ensure these structs are defined in internal/app/interfaces.go)
	taskServiceDeps := app.TaskServiceDeps{TaskRepo: taskRepo}
	agentServiceDeps := app.AgentServiceDeps{AgentRepo: agentRepo}
	toolServiceDeps := app.ToolServiceDeps{
		ToolRepo: toolRepo,     // Pass repo (can be nil if only runtime needed)
		Registry: toolRegistry, // Pass registry
	}
	platformServiceDeps := app.PlatformServiceDeps{
		TaskSvc:    nil, // TaskService needs to be created first
		AgentRtCli: agentRtClient,
		// Add AgentService/ToolService here if PlatformService needs them
	}

	// Create services, injecting dependencies (ensure New... functions exist in internal/app/)
	taskService := app.NewTaskService(taskServiceDeps)
	agentService := app.NewAgentService(agentServiceDeps)
	toolService := app.NewToolService(toolServiceDeps)
	platformServiceDeps.TaskSvc = taskService // Inject TaskService dependency into PlatformService deps
	platformService := app.NewPlatformService(platformServiceDeps)

	// --- WebSocket Manager ---
	// Pass the specific services the WS manager needs
	wsManager := ws.NewConnectionManager(platformService, taskService)

	// --- Register Example Agents (if repo is empty) ---
	registerExampleAgents(context.Background(), agentService)

	// --- API Router ---
	// Pass all services and the WS manager to the router
	// Ensure api.NewRouter signature matches these arguments
	router := api.NewRouter(
		platformService, // 1st: PlatformService
		taskService,     // 2nd: TaskService
		agentService,    // 3rd: AgentService
		toolService,     // 4th: ToolService
		wsManager,       // 5th: ws.Manager
	)

	// --- HTTP Server ---
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      router,           // Use the configured router
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Graceful Shutdown ---
	// Run server in a goroutine so that it doesn't block.
	go func() {
		log.Infof("Server starting on port %d", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Could not listen on %s: %v\n", server.Addr, err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("Shutting down server...")

	// The context is used to inform the server it has N seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Increased timeout
	defer cancel()

	// Shutdown WebSocket Manager first to stop accepting new connections & close existing
	log.Info("Closing WebSocket manager...")
	wsManager.Close() // Call the Close method

	// Shutdown HTTP server
	log.Info("Shutting down HTTP server...")
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connection if applicable
	// closeDatabase(db)

	log.Info("Server exiting")
}

// setupLogging configures the logger based on the loaded configuration.
func setupLogging(logLevel string) {
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Warnf("Invalid log level '%s', defaulting to 'info'. Error: %v", logLevel, err)
		level = log.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339, // More standard timestamp format
	})
	// Example: Output to a file as well
	// logFile, err := os.OpenFile("a2a-platform.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err == nil {
	//  log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	// } else {
	//  log.Info("Failed to log to file, using default stderr")
	// }

	log.Infof("Log level set to %s", level.String())
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
			// Assuming NewOpenAIClient exists and is implemented correctly
			client, err = llmclient.NewOpenAIClient(providerCfg.APIKey)
			if err != nil {
				log.Errorf("Failed to create OpenAI client: %v", err)
			}
		case "anthropic":
			// client, err = llmclient.NewAnthropicClient(providerCfg.APIKey) // Placeholder
			log.Warn("Anthropic client not implemented yet")
			// if err != nil { log.Errorf(...) }
		case "google":
			client, err = llmclient.NewGeminiClient(providerCfg.APIKey)
			if err != nil {
				log.Errorf("Failed to create Google Gemini client: %v", err)
			}
			// if err != nil { log.Errorf(...) }
		// Add cases for other providers (google, cohere, etc.)
		default:
			log.Warnf("Unsupported LLM provider configured: %s", providerName)
		}

		if client != nil && err == nil {
			clients = append(clients, client)
		}
	}
	log.Infof("Initialized %d LLM clients", len(clients))
	return clients
}

// --- Placeholder database functions (replace with real implementation if using SQL) ---
// func setupDatabase(dbURL string) *sql.DB { ... }
// func closeDatabase(db *sql.DB) { ... }

// registerExampleAgents adds some predefined agents if the repository is empty.
func registerExampleAgents(ctx context.Context, agentSvc app.AgentService) {
	log.Info("Checking for existing agents...")
	// Quick check - ideally AgentService would have List or Count method
	// For now, just try registering and ignore 'AlreadyExists' errors

	descEcho := "A simple agent that echoes back the input message."
	echoAgent := a2a.AgentCard{
		Name:        "EchoAgent",
		Description: &descEcho,
		URL:         "http://localhost:8081/a2a/agents/echo", // Dummy URL for example
		Version:     "1.0.0",
		Capabilities: a2a.AgentCapabilities{
			Streaming:             false,
			PushNotifications:     false,
			StateTransitionHistory: false,
		},
		Skills: []a2a.AgentSkill{
			{ID: "echo-text", Name: "Echo Text", Description: &descEcho},
		},
	}

	descCalc := "An agent capable of performing calculations using the platform's calculator tool."
	calculatorAgent := a2a.AgentCard{
		Name:        "CalculatorAgent",
		Description: &descCalc,
		URL:         "http://localhost:8081/a2a/agents/calculator", // Dummy URL
		Version:     "1.0.0",
		Capabilities: a2a.AgentCapabilities{
			Streaming:             false,
			PushNotifications:     false,
			StateTransitionHistory: false,
		},
		Skills: []a2a.AgentSkill{
			{
				ID:          "basic-math",
				Name:        "Basic Math",
				Description: ptrString("Performs addition, subtraction, multiplication, division."),
				Examples:    []string{"add 1 and 2", "what is 10 divided by 5?"},
			},
		},
	}

	agentsToRegister := []a2a.AgentCard{echoAgent, calculatorAgent}

	registeredCount := 0
	for _, card := range agentsToRegister {
		_, err := agentSvc.RegisterAgent(ctx, card)
		if err != nil {
			if errors.Is(err, memRepo.ErrAlreadyExists) {
				log.Debugf("Example agent '%s' already registered.", card.Name)
			} else {
				log.Warnf("Failed to register example agent '%s': %v", card.Name, err)
			}
		} else {
			log.Infof("Registered example agent: %s", card.Name)
			registeredCount++
		}
	}
}

func ptrString(s string) *string { return &s }