// internal/ws/manager.go
package ws

import (
	"fmt"
	"context"
	"encoding/json" // Added for handling incoming messages
	"errors"
	"net/http"
	"sync"
	"time" // Added for context timeout and write deadlines

	"github.com/gorilla/websocket"
	"github.com/kathir-ks/a2a-platform/internal/app" // Need app services potentially
	"github.com/kathir-ks/a2a-platform/pkg/a2a"     // Need A2A types for messages
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking for production
		log.Warnf("WebSocket CheckOrigin allowing request from origin: %s", r.Header.Get("Origin"))
		return true
	},
}

// Manager defines the interface for managing WebSocket connections and subscriptions.
type Manager interface {
	HandleConnection(w http.ResponseWriter, r *http.Request)
	Subscribe(conn *websocket.Conn, taskID string) error
	Unsubscribe(conn *websocket.Conn, taskID string) error
	SendToTask(ctx context.Context, taskID string, message any) error
	Close()
}

// client represents a single WebSocket client connection managed by the manager.
type client struct {
	conn *websocket.Conn
	send chan []byte // Buffered channel for outbound messages specific to this client
}

// connectionManager provides an in-memory implementation of the ws.Manager interface.
type connectionManager struct {
	subscriptions map[string]map[*client]struct{}
	clients       map[*client]bool
	mu            sync.RWMutex

	// References to core app services needed for handling incoming messages
	platformService app.PlatformService
	taskService     app.TaskService

	register   chan *client
	unregister chan *client
	broadcast  chan []byte // Optional
	stop       chan struct{}
}

// NewConnectionManager creates a new WebSocket connection manager.
// Accepts individual services directly (Option B from previous fixes).
func NewConnectionManager(ps app.PlatformService, ts app.TaskService) Manager {
	if ps == nil || ts == nil {
		log.Fatal("WebSocket Manager requires non-nil PlatformService and TaskService")
	}
	m := &connectionManager{
		subscriptions: make(map[string]map[*client]struct{}),
		clients:       make(map[*client]bool),
		register:      make(chan *client),
		unregister:    make(chan *client),
		broadcast:     make(chan []byte),
		stop:          make(chan struct{}),
		platformService: ps, // Assign injected services
		taskService:     ts, // Assign injected services
	}
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
			return

		case client := <-m.register:
			m.mu.Lock()
			m.clients[client] = true
			log.Debugf("WebSocket client registered: %s", client.conn.RemoteAddr())
			m.mu.Unlock()

		case client := <-m.unregister:
			m.mu.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				for taskID, subs := range m.subscriptions {
					if _, subscribed := subs[client]; subscribed {
						delete(m.subscriptions[taskID], client)
						if len(m.subscriptions[taskID]) == 0 {
							delete(m.subscriptions, taskID)
						}
						log.Debugf("Client %s unsubscribed from task %s", client.conn.RemoteAddr(), taskID)
					}
				}
				close(client.send)
				log.Debugf("WebSocket client unregistered: %s", client.conn.RemoteAddr())
			}
			m.mu.Unlock()
		}
	}
}

// HandleConnection implements the Manager interface.
func (m *connectionManager) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade WebSocket connection: %v", err)
		return
	}
	log.Infof("WebSocket connection established: %s", conn.RemoteAddr())

	client := &client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	m.register <- client

	go m.writePump(client)
	go m.readPump(client)
}

// readPump pumps messages from the WebSocket connection to the manager/app logic.
func (m *connectionManager) readPump(c *client) {
	defer func() {
		m.unregister <- c
		c.conn.Close()
		log.Debugf("WebSocket readPump finished for %s", c.conn.RemoteAddr())
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil
	})

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warnf("WebSocket read error for %s: %v", c.conn.RemoteAddr(), err)
			} else {
				log.Debugf("WebSocket closed normally for %s: %v", c.conn.RemoteAddr(), err)
			}
			break
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			log.Debugf("Received WebSocket message from %s (type: %d, size: %d)", c.conn.RemoteAddr(), messageType, len(message))
			m.handleIncomingWSMessage(c, message) // Process the message
		} else {
			log.Debugf("Ignoring WebSocket message type %d from %s", messageType, c.conn.RemoteAddr())
		}
	}
}

// writePump pumps messages from the client's send channel to the WebSocket connection.
func (m *connectionManager) writePump(c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		log.Debugf("WebSocket writePump finished for %s", c.conn.RemoteAddr())
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				log.Debugf("Client send channel closed for %s, sending close message", c.conn.RemoteAddr())
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Errorf("Error getting writer for WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return
			}
			_, err = w.Write(message)
			if err != nil {
				log.Errorf("Error writing message to WebSocket %s: %v", c.conn.RemoteAddr(), err)
				// Don't return immediately, try closing writer first
			}

			if err := w.Close(); err != nil {
				log.Errorf("Error closing writer for WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Warnf("Error sending ping to WebSocket %s: %v", c.conn.RemoteAddr(), err)
				return
			}
			log.Tracef("Sent ping to %s", c.conn.RemoteAddr())
		}
	}
}

// handleIncomingWSMessage processes messages received from a WebSocket client.
func (m *connectionManager) handleIncomingWSMessage(c *client, message []byte) {
	var rawReq struct {
		a2a.JSONRPCMessage
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}

	if err := json.Unmarshal(message, &rawReq); err != nil {
		log.Warnf("Failed to decode incoming WS message from %s: %v", c.conn.RemoteAddr(), err)
		errResp := a2a.NewParseError(err.Error())
		// Correctly initialize embedded message
		m.sendToClient(c, a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Use ID from rawReq if available
			Error:          errResp,
		})
		return
	}

	_, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Keep context
	defer cancel()

	// Route based on method
	switch rawReq.Method {
	case a2a.MethodSendTaskSubscribe:
		var params a2a.TaskSendParams
		if err := json.Unmarshal(rawReq.Params, &params); err != nil {
			errResp := a2a.NewInvalidParamsError(err.Error())
			m.sendToClient(c, a2a.JSONRPCResponse{
				JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
				Error:          errResp,
			})
			return
		}
		log.Infof("WS Client %s requests subscription via SendTaskSubscribe for task %s", c.conn.RemoteAddr(), params.ID)

		if subErr := m.Subscribe(c.conn, params.ID); subErr != nil {
			log.Errorf("Error subscribing client %s to task %s: %v", c.conn.RemoteAddr(), params.ID, subErr)
			errResp := a2a.NewInternalError("failed to subscribe to task stream")
			m.sendToClient(c, a2a.JSONRPCResponse{
				JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
				Error:          errResp,
			})
			return
		}

		// --- Call PlatformService (Placeholder - pass ctx if needed) ---
		// eventChan, rpcErr := m.platformService.HandleSendTaskSubscribe(ctx, params)
		// if rpcErr != nil { ... }
		log.Warn("PlatformService.HandleSendTaskSubscribe integration not fully implemented")

		// Send acknowledgement response
		m.sendToClient(c, a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
			Result:         map[string]string{"status": "subscribed"},
		})

	case a2a.MethodResubscribeTask:
		var params a2a.TaskQueryParams
		if err := json.Unmarshal(rawReq.Params, &params); err != nil {
			errResp := a2a.NewInvalidParamsError(err.Error())
			m.sendToClient(c, a2a.JSONRPCResponse{
				JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
				Error:          errResp,
			})
			return
		}
		log.Infof("WS Client %s requests ResubscribeTask for task %s", c.conn.RemoteAddr(), params.ID)

		if subErr := m.Subscribe(c.conn, params.ID); subErr != nil {
			log.Errorf("Error resubscribing client %s to task %s: %v", c.conn.RemoteAddr(), params.ID, subErr)
			errResp := a2a.NewInternalError("failed to resubscribe to task stream")
			m.sendToClient(c, a2a.JSONRPCResponse{
				JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
				Error:          errResp,
			})
			return
		}

		// --- Call PlatformService (Placeholder - pass ctx if needed) ---
		// _, rpcErr := m.platformService.HandleResubscribeTask(ctx, params)
		// if rpcErr != nil { ... }
		log.Warn("PlatformService.HandleResubscribeTask integration not fully implemented")

		// Send ack response
		m.sendToClient(c, a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
			Result:         map[string]string{"status": "resubscribed"},
		})

	default:
		log.Warnf("Received unsupported method '%s' over WebSocket from %s", rawReq.Method, c.conn.RemoteAddr())
		errResp := a2a.NewMethodNotFoundError(nil)
		m.sendToClient(c, a2a.JSONRPCResponse{
			JSONRPCMessage: a2a.JSONRPCMessage{JSONRPC: a2a.JSONRPCVersion, ID: rawReq.ID}, // Fix
			Error:          errResp,
		})
	}
}

// Subscribe implements the Manager interface.
func (m *connectionManager) Subscribe(conn *websocket.Conn, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	if _, ok := m.subscriptions[taskID]; !ok {
		m.subscriptions[taskID] = make(map[*client]struct{})
	}

	if _, already := m.subscriptions[taskID][targetClient]; already {
		log.Debugf("Client %s already subscribed to task %s", conn.RemoteAddr(), taskID)
		return nil
	}

	m.subscriptions[taskID][targetClient] = struct{}{}
	log.Infof("Client %s subscribed to task %s", conn.RemoteAddr(), taskID)
	return nil
}

// Unsubscribe implements the Manager interface.
func (m *connectionManager) Unsubscribe(conn *websocket.Conn, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	if subs, ok := m.subscriptions[taskID]; ok {
		if _, subscribed := subs[targetClient]; subscribed {
			delete(m.subscriptions[taskID], targetClient)
			log.Infof("Client %s unsubscribed from task %s", conn.RemoteAddr(), taskID)
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
	m.mu.RLock()
	subs, ok := m.subscriptions[taskID]
	if !ok || len(subs) == 0 {
		m.mu.RUnlock()
		log.Debugf("No subscribers found for task %s, message not sent", taskID)
		return nil
	}

	clientsToSend := make([]*client, 0, len(subs))
	for c := range subs {
		clientsToSend = append(clientsToSend, c)
	}
	m.mu.RUnlock()

	payload, err := json.Marshal(message)
	if err != nil {
		log.Errorf("Failed to marshal message for task %s: %v", taskID, err)
		return fmt.Errorf("failed to marshal WebSocket message: %w", err)
	}

	log.Debugf("Sending message to %d subscriber(s) for task %s", len(clientsToSend), taskID)

	var wg sync.WaitGroup
	for _, c := range clientsToSend {
		wg.Add(1)
		go func(client *client) {
			defer wg.Done()
			select {
			case client.send <- payload:
			case <-time.After(1 * time.Second):
				log.Warnf("Timeout queuing message for client %s on task %s", client.conn.RemoteAddr(), taskID)
			case <-m.stop:
				log.Debugf("Manager stopping, aborting send to client %s", client.conn.RemoteAddr())
			}
		}(c)
	}
	wg.Wait()

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
	case <-time.After(1 * time.Second):
		log.Warnf("Timeout queuing direct message for client %s", c.conn.RemoteAddr())
	case <-m.stop:
		log.Debugf("Manager stopping, aborting direct send to client %s", c.conn.RemoteAddr())
	}
}

// Close implements the Manager interface.
func (m *connectionManager) Close() {
	log.Info("Closing WebSocket Manager...")
	close(m.stop)
}

// cleanupAllConnections closes all client connections. Called when manager stops.
func (m *connectionManager) cleanupAllConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Infof("Closing %d active WebSocket connections...", len(m.clients))
	for c := range m.clients {
		c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
		close(c.send)
		c.conn.Close()
		delete(m.clients, c)
	}
	m.subscriptions = make(map[string]map[*client]struct{}) // Clear subscriptions
	log.Info("All WebSocket connections closed.")
}

// Define WebSocket constants
const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1024 * 10
)