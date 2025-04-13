// internal/ws/manager.go
package ws

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/kathir-ks/a2a-platform/internal/app" // Need app services potentially
	"github.com/kathir-ks/a2a-platform/pkg/a2a"     // Need A2A types for messages
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CheckOrigin prevents CSRF attacks. In production, list allowed origins.
	// For development, you might allow all, but be careful.
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking for production
		log.Warnf("WebSocket CheckOrigin allowing request from origin: %s", r.Header.Get("Origin"))
		return true
	},
}

// Manager defines the interface for managing WebSocket connections and subscriptions.
type Manager interface {
	// HandleConnection upgrades an HTTP request to a WebSocket connection and manages its lifecycle.
	HandleConnection(w http.ResponseWriter, r *http.Request)
	// Subscribe associates a connection with a task stream.
	Subscribe(conn *websocket.Conn, taskID string) error
	// Unsubscribe removes a connection's association with a task stream.
	Unsubscribe(conn *websocket.Conn, taskID string) error
	// SendToTask sends a message (like TaskStatusUpdateEvent) to all connections subscribed to a specific taskID.
	SendToTask(ctx context.Context, taskID string, message any) error // 'any' could be TaskStatusUpdateEvent, TaskArtifactUpdateEvent, JSONRPCError
	// Close terminates the manager and all connections.
	Close()
}

// client represents a single WebSocket client connection managed by the manager.
type client struct {
	conn *websocket.Conn
	send chan []byte // Buffered channel for outbound messages specific to this client
	// You might add user ID, session ID etc. here if needed for auth/filtering
}

// connectionManager provides an in-memory implementation of the ws.Manager interface.
type connectionManager struct {
	// Maps Task ID -> map[client pointer] -> empty struct (set for efficient add/delete)
	subscriptions map[string]map[*client]struct{}
	clients       map[*client]bool // Set of all active clients
	mu            sync.RWMutex     // Protects subscriptions and clients maps

	appServices *app.Services // Reference to core app services if needed for handling incoming messages

	register   chan *client // Channel for new client registrations
	unregister chan *client // Channel for client unregistrations
	broadcast  chan []byte  // Optional: Channel for broadcasting to ALL clients (use with caution)
	stop       chan struct{} // Channel to signal manager shutdown
}

// AppServices is a simple struct to group app service dependencies for the WS manager
type AppServices struct {
	PlatformService app.PlatformService
	TaskService     app.TaskService
	// Add other services if needed by WS handlers
}

// NewConnectionManager creates a new WebSocket connection manager.
func NewConnectionManager(appSvcs *AppServices) Manager {
	m := &connectionManager{
		subscriptions: make(map[string]map[*client]struct{}),
		clients:       make(map[*client]bool),
		register:      make(chan *client),
		unregister:    make(chan *client),
		broadcast:     make(chan []byte),
		stop:          make(chan struct{}),
		appServices:   appSvcs,
	}
	// Start the manager's central processing loop in a goroutine
	go m.run()
	return m
}

// run is the central event loop for the manager.
func (m *connectionManager) run() {
	log.Info("WebSocket Manager started")
	defer log.Info("WebSocket Manager stopped")

	for {
		select {
		case <-m.stop:
			m.cleanupAllConnections()
			return // Exit the loop

		case client := <-m.register:
			m.mu.Lock()
			m.clients[client] = true
			log.Debugf("WebSocket client registered: %s", client.conn.RemoteAddr())
			m.mu.Unlock()

		case client := <-m.unregister:
			m.mu.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				// Also remove from all subscriptions
				for taskID, subs := range m.subscriptions {
					if _, subscribed := subs[client]; subscribed {
						delete(m.subscriptions[taskID], client)
						if len(m.subscriptions[taskID]) == 0 {
							delete(m.subscriptions, taskID) // Clean up empty task subscription maps
						}
						log.Debugf("Client %s unsubscribed from task %s", client.conn.RemoteAddr(), taskID)
					}
				}
				close(client.send) // Close the client's send channel
				log.Debugf("WebSocket client unregistered: %s", client.conn.RemoteAddr())
			}
			m.mu.Unlock()

			// Optional: Broadcast message handling (not used for task-specific sends)
			// case message := <-m.broadcast:
			//  m.mu.RLock()
			//  for client := range m.clients {
			//      select {
			//      case client.send <- message:
			//      default: // Don't block if send buffer is full
			//          close(client.send)
			//          delete(m.clients, client) // Consider unregistering slow clients
			//      }
			//  }
			//  m.mu.RUnlock()
		}
	}
}

// HandleConnection implements the Manager interface.
func (m *connectionManager) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // Upgrade HTTP connection
	if err != nil {
		log.Errorf("Failed to upgrade WebSocket connection: %v", err)
		// Upgrade likely wrote an error response already
		return
	}
	log.Infof("WebSocket connection established: %s", conn.RemoteAddr())

	// Create a new client representation
	client := &client{
		conn: conn,
		send: make(chan []byte, 256), // Buffered channel for outbound messages
	}

	// Register the new client with the manager
	m.register <- client

	// Start goroutines for reading and writing messages for this client
	go m.writePump(client)
	go m.readPump(client)

	// Note: The client is now running until readPump or writePump exits,
	// which triggers unregistration in the `defer` statements within those functions.
}

// readPump pumps messages from the WebSocket connection to the manager/app logic.
func (m *connectionManager) readPump(c *client) {
	// Ensure unregistration and connection closure on exit
	defer func() {
		m.unregister <- c
		c.conn.Close()
		log.Debugf("WebSocket readPump finished for %s", c.conn.RemoteAddr())
	}()

	// Set read limits and deadlines
	c.conn.SetReadLimit(maxMessageSize) // Define maxMessageSize somewhere (e.g., 512)
	c.conn.SetReadDeadline(time.Now().Add(pongWait)) // Requires writePump sending pings

	// Set up pong handler to keep connection alive
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil
	})

	// Loop reading messages from the client
	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warnf("WebSocket read error for %s: %v", c.conn.RemoteAddr(), err)
			} else {
				log.Debugf("WebSocket closed normally for %s: %v", c.conn.RemoteAddr(), err)
			}
			break // Exit loop on error or closure
		}

        if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
		    log.Debugf("Received WebSocket message from %s (type: %d, size: %d)", c.conn.RemoteAddr(), messageType, len(message))
		    // --- Handle Incoming Message ---
		    m.handleIncomingWSMessage(c, message)
        } else {
             log.Debugf("Ignoring WebSocket message type %d from %s", messageType, c.conn.RemoteAddr())
        }
	}
}

// writePump pumps messages from the client's send channel to the WebSocket connection.
func (m *connectionManager) writePump(c *client) {
	ticker := time.NewTicker(pingPeriod) // Send pings periodically

	// Ensure ticker stop and connection closure on exit
	defer func() {
		ticker.Stop()
		c.conn.Close() // Close connection if writing fails
		log.Debugf("WebSocket writePump finished for %s", c.conn.RemoteAddr())
		// Unregistration is handled by readPump defer normally, but could be forced here too if needed
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait)) // Set deadline for writing
			if !ok {
				// The manager closed the channel.
				log.Debugf("Client send channel closed for %s, sending close message", c.conn.RemoteAddr())
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return // Exit goroutine
			}

			w, err := c.conn.NextWriter(websocket.TextMessage) // Assuming text messages
			if err != nil {
				log.Errorf("Error getting writer for WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return // Exit goroutine
			}
			_, err = w.Write(message)
			if err != nil {
				log.Errorf("Error writing message to WebSocket %s: %v", c.conn.RemoteAddr(), err)
				// Don't return immediately, try closing writer first
			}

			// Add queued chat messages to the current WebSocket message, if any.
			// This can improve efficiency by batching messages. (Optional optimization)
			// n := len(c.send)
			// for i := 0; i < n; i++ {
			//     w.Write(newline) // Or use appropriate framing/delimiter
			//     w.Write(<-c.send)
			// }

			if err := w.Close(); err != nil {
				log.Errorf("Error closing writer for WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return // Exit goroutine
			}

		case <-ticker.C:
			// Send ping message
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Warnf("Error sending ping to WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return // Assume connection is dead
			}
			log.Tracef("Sent ping to %s", c.conn.RemoteAddr())
		}
	}
}

// handleIncomingWSMessage processes messages received from a WebSocket client.
func (m *connectionManager) handleIncomingWSMessage(c *client, message []byte) {
	// Decode the message as a generic JSON-RPC request
	var req a2a.JSONRPCRequest
	var rawReq struct {
		a2a.JSONRPCMessage
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(message, &rawReq); err != nil {
		log.Warnf("Failed to decode incoming WS message from %s: %v", c.conn.RemoteAddr(), err)
		// Send parse error back to client
		errResp := a2a.NewParseError(err.Error())
		m.sendToClient(c, a2a.JSONRPCResponse{Error: errResp}) // Use helper to send error
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Context for handling this request
	defer cancel()

	// Route based on method
	switch rawReq.Method {
	case a2a.MethodSendTaskSubscribe:
		var params a2a.TaskSendParams
		if err := json.Unmarshal(rawReq.Params, ¶ms); err != nil {
			errResp := a2a.NewInvalidParamsError(err.Error())
			m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: errResp})
			return
		}
		log.Infof("WS Client %s requests subscription via SendTaskSubscribe for task %s", c.conn.RemoteAddr(), params.ID)
		// 1. Subscribe the connection to the task ID
		if subErr := m.Subscribe(c.conn, params.ID); subErr != nil {
			// Handle subscription error (e.g., already subscribed?)
			log.Errorf("Error subscribing client %s to task %s: %v", c.conn.RemoteAddr(), params.ID, subErr)
            errResp := a2a.NewInternalError("failed to subscribe to task stream")
            m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: errResp})
			return
		}
        // 2. Call PlatformService to handle the actual task sending logic
		// This service method needs to be designed to return a channel or handle async updates
        // For now, let's assume PlatformService handles it and will use m.SendToTask later.
		// eventChan, rpcErr := m.appServices.PlatformService.HandleSendTaskSubscribe(ctx, params)
		// if rpcErr != nil {
		//     m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: rpcErr})
        //     // Optionally unsubscribe on failure?
		//     m.Unsubscribe(c.conn, params.ID)
		//     return
		// }
        // // Forward initial success response (if any)
        // // Start goroutine to forward events from eventChan to this client? NO - SendToTask does this.
        log.Warn("PlatformService.HandleSendTaskSubscribe integration not fully implemented")
        // Send an acknowledgement response (maybe just empty success for now)
        m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Result: map[string]string{"status": "subscribed"}})


	case a2a.MethodResubscribeTask:
		 var params a2a.TaskQueryParams
		if err := json.Unmarshal(rawReq.Params, ¶ms); err != nil {
			errResp := a2a.NewInvalidParamsError(err.Error())
			m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: errResp})
			return
		}
        log.Infof("WS Client %s requests ResubscribeTask for task %s", c.conn.RemoteAddr(), params.ID)
		// 1. Subscribe the connection
        if subErr := m.Subscribe(c.conn, params.ID); subErr != nil {
			log.Errorf("Error resubscribing client %s to task %s: %v", c.conn.RemoteAddr(), params.ID, subErr)
            errResp := a2a.NewInternalError("failed to resubscribe to task stream")
            m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: errResp})
			return
		}
        // 2. Call PlatformService? Or just subscribe and let existing updates flow?
        // Maybe PlatformService.HandleResubscribeTask needs to fetch history or trigger agent?
        log.Warn("PlatformService.HandleResubscribeTask integration not fully implemented")
        // Send ack response
        m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Result: map[string]string{"status": "resubscribed"}})


	// TODO: Handle Unsubscribe method?
	// case "tasks/unsubscribe": ... m.Unsubscribe(...) ...

	default:
		log.Warnf("Received unsupported method '%s' over WebSocket from %s", rawReq.Method, c.conn.RemoteAddr())
		errResp := a2a.NewMethodNotFoundError(nil)
		m.sendToClient(c, a2a.JSONRPCResponse{ID: rawReq.ID, Error: errResp})
	}
}

// Subscribe implements the Manager interface.
func (m *connectionManager) Subscribe(conn *websocket.Conn, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the client object associated with the connection
	var targetClient *client
	for c := range m.clients {
		if c.conn == conn {
			targetClient = c
			break
		}
	}
	if targetClient == nil {
		return errors.New("cannot subscribe: connection not registered")
	}

	// Get or create the subscription map for the task
	if _, ok := m.subscriptions[taskID]; !ok {
		m.subscriptions[taskID] = make(map[*client]struct{})
	}

	// Add client to the task's subscription set
	if _, already := m.subscriptions[taskID][targetClient]; already {
		log.Debugf("Client %s already subscribed to task %s", conn.RemoteAddr(), taskID)
		return nil // Not an error to subscribe again
	}

	m.subscriptions[taskID][targetClient] = struct{}{}
	log.Infof("Client %s subscribed to task %s", conn.RemoteAddr(), taskID)
	return nil
}

// Unsubscribe implements the Manager interface.
func (m *connectionManager) Unsubscribe(conn *websocket.Conn, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the client object
	var targetClient *client
	for c := range m.clients {
		if c.conn == conn {
			targetClient = c
			break
		}
	}
	if targetClient == nil {
		return errors.New("cannot unsubscribe: connection not registered")
	}

	// Remove client from the task's subscription set
	if subs, ok := m.subscriptions[taskID]; ok {
		if _, subscribed := subs[targetClient]; subscribed {
			delete(m.subscriptions[taskID], targetClient)
			log.Infof("Client %s unsubscribed from task %s", conn.RemoteAddr(), taskID)
			// Clean up map if last subscriber leaves
			if len(m.subscriptions[taskID]) == 0 {
				delete(m.subscriptions, taskID)
				log.Debugf("Removed empty subscription map for task %s", taskID)
			}
			return nil
		}
	}
	log.Warnf("Client %s attempted to unsubscribe from task %s, but was not subscribed", conn.RemoteAddr(), taskID)
	return errors.New("client not subscribed to this task")
}


// SendToTask implements the Manager interface.
func (m *connectionManager) SendToTask(ctx context.Context, taskID string, message any) error {
	m.mu.RLock() // Read lock to access subscriptions
	subs, ok := m.subscriptions[taskID]
	if !ok || len(subs) == 0 {
		m.mu.RUnlock()
		log.Debugf("No subscribers found for task %s, message not sent", taskID)
		return nil // Not an error if no one is listening
	}

	// Create a list of clients to send to under read lock
	clientsToSend := make([]*client, 0, len(subs))
	for c := range subs {
		clientsToSend = append(clientsToSend, c)
	}
	m.mu.RUnlock() // Release lock before potentially long-running send operations

	// Marshal the message once
	payload, err := json.Marshal(message)
	if err != nil {
		log.Errorf("Failed to marshal message for task %s: %v", taskID, err)
		return fmt.Errorf("failed to marshal WebSocket message: %w", err)
	}

	log.Debugf("Sending message to %d subscriber(s) for task %s", len(clientsToSend), taskID)

	// Send to each client concurrently (or sequentially if preferred)
	var wg sync.WaitGroup
	for _, c := range clientsToSend {
		wg.Add(1)
		go func(client *client) {
			defer wg.Done()
			select {
			case client.send <- payload:
				// Message queued successfully
			case <-time.After(1 * time.Second): // Timeout for queuing
				log.Warnf("Timeout queuing message for client %s on task %s", client.conn.RemoteAddr(), taskID)
				// Optionally unregister slow clients here if needed
			case <-m.stop: // Check if manager is stopping
				log.Debugf("Manager stopping, aborting send to client %s", client.conn.RemoteAddr())
			}
		}(c) // Pass client as arg to goroutine
	}
	wg.Wait() // Wait for all send attempts (or timeouts)

	return nil
}

// sendToClient is a helper to marshal and queue a message for a specific client.
func (m *connectionManager) sendToClient(c *client, message any) {
     payload, err := json.Marshal(message)
	if err != nil {
		log.Errorf("Failed to marshal message for client %s: %v", c.conn.RemoteAddr(), err)
		return
	}
    select {
        case c.send <- payload:
            // Queued
        case <-time.After(1 * time.Second):
            log.Warnf("Timeout queuing direct message for client %s", c.conn.RemoteAddr())
        case <-m.stop:
            log.Debugf("Manager stopping, aborting direct send to client %s", c.conn.RemoteAddr())
    }
}


// Close implements the Manager interface.
func (m *connectionManager) Close() {
	log.Info("Closing WebSocket Manager...")
	close(m.stop) // Signal the run loop to stop
	// Wait briefly for cleanup? The run loop handles cleanup now.
}

// cleanupAllConnections closes all client connections. Called when manager stops.
func (m *connectionManager) cleanupAllConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Infof("Closing %d active WebSocket connections...", len(m.clients))
	for c := range m.clients {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
		close(c.send)
		c.conn.Close() // Force close
		delete(m.clients, c)
	}
	m.subscriptions = make(map[string]map[*client]struct{}) // Clear subscriptions
	log.Info("All WebSocket connections closed.")
}


// Define WebSocket constants
const (
	writeWait      = 10 * time.Second    // Time allowed to write a message to the peer.
	pongWait       = 60 * time.Second    // Time allowed to read the next pong message from the peer.
	pingPeriod     = (pongWait * 9) / 10 // Send pings to peer with this period. Must be less than pongWait.
	maxMessageSize = 1024 * 10           // Maximum message size allowed from peer.
)

// Variables for newline (optional, for message batching in writePump)
// var (
// 	newline = []byte{'\n'}
// 	space   = []byte{' '}
// )