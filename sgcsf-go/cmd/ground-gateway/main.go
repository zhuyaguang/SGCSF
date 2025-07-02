package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
	"github.com/sgcsf/sgcsf-go/pkg/transport"
)

// GroundGateway manages all communication between ground apps and satellites
type GroundGateway struct {
	// SGCSF server for satellite connections
	sgcsfServer *transport.QUICTransport
	
	// Local message router
	messageRouter *MessageRouter
	
	// Subscription management
	subscriptionMgr *SubscriptionManager
	
	// HTTP API for ground applications
	httpServer *http.Server
	
	// Configuration
	config *GroundGatewayConfig
	
	// Connected satellites
	satellites map[string]*SatelliteConnection
	mutex      sync.RWMutex
	stopChan   chan struct{}
}

// GroundGatewayConfig represents ground gateway configuration
type GroundGatewayConfig struct {
	ListenAddr     string        `json:"listen_addr"`
	HTTPPort       string        `json:"http_port"`
	MaxSatellites  int           `json:"max_satellites"`
	MessageTTL     time.Duration `json:"message_ttl"`
	QueueSize      int           `json:"queue_size"`
}

// MessageRouter routes messages between satellites and ground applications
type MessageRouter struct {
	routes map[string][]string // topic -> satellite IDs
	mutex  sync.RWMutex
}

// SubscriptionManager manages subscriptions from ground applications
type SubscriptionManager struct {
	subscriptions map[string][]types.MessageHandler // topic -> handlers
	mutex         sync.RWMutex
}

// SatelliteConnection represents a connected satellite
type SatelliteConnection struct {
	ID           string
	Connection   *transport.QUICConnection
	LastSeen     time.Time
	Status       string
	MessageQueue chan *types.SGCSFMessage
}

func main() {
	fmt.Println("🌍 Starting SGCSF Ground Gateway")

	// Load configuration
	config := &GroundGatewayConfig{
		ListenAddr:    getEnv("LISTEN_ADDR", ":7000"),
		HTTPPort:      getEnv("HTTP_PORT", ":8080"),
		MaxSatellites: getEnvInt("MAX_SATELLITES", 100),
		MessageTTL:    time.Duration(getEnvInt("MESSAGE_TTL_MINUTES", 60)) * time.Minute,
		QueueSize:     getEnvInt("QUEUE_SIZE", 10000),
	}

	// Create ground gateway
	gateway, err := NewGroundGateway(config)
	if err != nil {
		log.Fatalf("Failed to create ground gateway: %v", err)
	}

	// Start the gateway
	ctx := context.Background()
	err = gateway.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}

	fmt.Printf("✅ Ground Gateway started\n")
	fmt.Printf("📡 SGCSF Server listening on %s\n", config.ListenAddr)
	fmt.Printf("🌐 HTTP API listening on %s\n", config.HTTPPort)
	fmt.Printf("🛰️  Ready for satellite connections (max: %d)\n", config.MaxSatellites)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to shutdown the gateway...")
	<-sigChan

	fmt.Println("\n🛑 Shutdown signal received. Stopping gateway...")
	gateway.Stop()
	fmt.Println("✅ Ground Gateway shut down gracefully")
}

// NewGroundGateway creates a new ground gateway
func NewGroundGateway(config *GroundGatewayConfig) (*GroundGateway, error) {
	// Create QUIC transport for satellite connections
	quicConfig := &transport.QUICConfig{
		ListenAddr:       config.ListenAddr,
		MaxStreams:       1000,
		KeepAlive:        30 * time.Second,
		HandshakeTimeout: 10 * time.Second,
		IdleTimeout:      5 * time.Minute,
		MaxPacketSize:    1350,
		EnableRetries:    true,
		MaxRetries:       3,
	}

	sgcsfServer := transport.NewQUICTransport(quicConfig)

	// Create message router
	messageRouter := &MessageRouter{
		routes: make(map[string][]string),
	}

	// Create subscription manager
	subscriptionMgr := &SubscriptionManager{
		subscriptions: make(map[string][]types.MessageHandler),
	}

	gateway := &GroundGateway{
		sgcsfServer:     sgcsfServer,
		messageRouter:   messageRouter,
		subscriptionMgr: subscriptionMgr,
		config:          config,
		satellites:      make(map[string]*SatelliteConnection),
		stopChan:        make(chan struct{}),
	}

	return gateway, nil
}

// Start starts the ground gateway
func (gg *GroundGateway) Start(ctx context.Context) error {
	// Start SGCSF server for satellite connections
	err := gg.sgcsfServer.Listen(ctx)
	if err != nil {
		return fmt.Errorf("failed to start SGCSF server: %v", err)
	}

	// Start HTTP API server for ground applications
	go gg.startHTTPServer()

	// Start satellite connection manager
	go gg.manageSatelliteConnections(ctx)

	// Start message processing
	go gg.processMessages()

	// Start satellite health monitoring
	go gg.monitorSatellites()

	return nil
}

// Stop stops the ground gateway
func (gg *GroundGateway) Stop() {
	close(gg.stopChan)
	
	// Stop SGCSF server
	if gg.sgcsfServer != nil {
		gg.sgcsfServer.Stop()
	}
	
	// Stop HTTP server
	if gg.httpServer != nil {
		gg.httpServer.Shutdown(context.Background())
	}
}

// startHTTPServer starts HTTP API for ground applications
func (gg *GroundGateway) startHTTPServer() {
	mux := http.NewServeMux()

	// Publish to satellites
	mux.HandleFunc("/api/publish", gg.handlePublish)
	
	// Subscribe to satellite messages
	mux.HandleFunc("/api/subscribe", gg.handleSubscribe)
	
	// Satellite management
	mux.HandleFunc("/api/satellites", gg.handleSatellites)
	mux.HandleFunc("/api/satellites/", gg.handleSatelliteDetail)
	
	// Command satellites
	mux.HandleFunc("/api/command", gg.handleCommand)
	
	// System status
	mux.HandleFunc("/api/status", gg.handleSystemStatus)
	
	// Health check
	mux.HandleFunc("/health", gg.handleHealth)
	
	// Metrics for monitoring
	mux.HandleFunc("/metrics", gg.handleMetrics)

	gg.httpServer = &http.Server{
		Addr:    gg.config.HTTPPort,
		Handler: mux,
	}

	log.Printf("🌐 HTTP API server started on %s", gg.config.HTTPPort)
	err := gg.httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Printf("HTTP server error: %v", err)
	}
}

// handlePublish handles publish requests from ground applications
func (gg *GroundGateway) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Topic      string      `json:"topic"`
		Payload    interface{} `json:"payload"`
		Satellites []string    `json:"satellites,omitempty"` // specific satellites, empty = all
		Priority   string      `json:"priority,omitempty"`
		TTL        int         `json:"ttl_minutes,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create SGCSF message
	priority := types.PriorityNormal
	if req.Priority == "high" {
		priority = types.PriorityHigh
	} else if req.Priority == "critical" {
		priority = types.PriorityCritical
	}

	ttl := time.Duration(req.TTL) * time.Minute
	if ttl == 0 {
		ttl = gg.config.MessageTTL
	}

	message := core.NewMessage().
		Topic(req.Topic).
		Type(types.MessageTypeAsync).
		Priority(priority).
		TTL(ttl).
		Source("ground-control").
		Payload(req.Payload).
		Build()

	// Route message to satellites
	targetSatellites := req.Satellites
	if len(targetSatellites) == 0 {
		// Send to all connected satellites
		gg.mutex.RLock()
		for satID := range gg.satellites {
			targetSatellites = append(targetSatellites, satID)
		}
		gg.mutex.RUnlock()
	}

	results := make(map[string]string)
	for _, satID := range targetSatellites {
		err := gg.sendToSatellite(satID, message)
		if err != nil {
			results[satID] = fmt.Sprintf("failed: %v", err)
		} else {
			results[satID] = "sent"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "processed",
		"message_id": message.ID,
		"results":    results,
	})
}

// handleSubscribe handles subscription requests from ground applications
func (gg *GroundGateway) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Topics []string `json:"topics"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create subscription
	subscriptionID := types.GenerateClientID("sub")
	
	// For demo purposes, return subscription ID
	// In real implementation, this would set up WebSocket or SSE
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subscription_id": subscriptionID,
		"topics":         req.Topics,
		"status":         "subscribed",
		"note":          "Use WebSocket endpoint /ws for real-time messages",
	})
}

// handleSatellites returns list of connected satellites
func (gg *GroundGateway) handleSatellites(w http.ResponseWriter, r *http.Request) {
	gg.mutex.RLock()
	satellites := make([]map[string]interface{}, 0, len(gg.satellites))
	
	for id, sat := range gg.satellites {
		satellites = append(satellites, map[string]interface{}{
			"id":          id,
			"status":      sat.Status,
			"last_seen":   sat.LastSeen.Unix(),
			"queue_size":  len(sat.MessageQueue),
		})
	}
	gg.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"satellites": satellites,
		"total":     len(satellites),
	})
}

// handleSatelliteDetail returns detailed info for a specific satellite
func (gg *GroundGateway) handleSatelliteDetail(w http.ResponseWriter, r *http.Request) {
	satID := r.URL.Path[len("/api/satellites/"):]
	
	gg.mutex.RLock()
	sat, exists := gg.satellites[satID]
	gg.mutex.RUnlock()
	
	if !exists {
		http.Error(w, "Satellite not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          satID,
		"status":      sat.Status,
		"last_seen":   sat.LastSeen.Unix(),
		"queue_size":  len(sat.MessageQueue),
		"connection":  sat.Connection.GetRemoteAddr(),
	})
}

// handleCommand sends commands to satellites
func (gg *GroundGateway) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SatelliteID string      `json:"satellite_id"`
		Command     string      `json:"command"`
		Parameters  interface{} `json:"parameters,omitempty"`
		Timeout     int         `json:"timeout_seconds,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create command message
	commandTopic := fmt.Sprintf("/satellite/%s/commands/%s", req.SatelliteID, req.Command)
	
	message := core.NewMessage().
		Topic(commandTopic).
		Type(types.MessageTypeSync).
		Priority(types.PriorityHigh).
		TTL(time.Duration(req.Timeout) * time.Second).
		Source("ground-control").
		Payload(map[string]interface{}{
			"command":    req.Command,
			"parameters": req.Parameters,
			"timestamp":  time.Now().Unix(),
		}).
		Build()

	// Send command
	err = gg.sendToSatellite(req.SatelliteID, message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "sent",
		"command_id": message.ID,
		"satellite":  req.SatelliteID,
	})
}

// handleSystemStatus returns overall system status
func (gg *GroundGateway) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	gg.mutex.RLock()
	connectedSats := len(gg.satellites)
	gg.mutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "running",
		"connected_satellites": connectedSats,
		"max_satellites":      gg.config.MaxSatellites,
		"uptime":             time.Now().Unix(),
		"message_ttl":        gg.config.MessageTTL.String(),
	})
}

// handleHealth returns health status
func (gg *GroundGateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleMetrics returns Prometheus metrics
func (gg *GroundGateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	gg.mutex.RLock()
	connectedSats := len(gg.satellites)
	gg.mutex.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP sgcsf_connected_satellites Number of connected satellites\n")
	fmt.Fprintf(w, "# TYPE sgcsf_connected_satellites gauge\n")
	fmt.Fprintf(w, "sgcsf_connected_satellites %d\n", connectedSats)
	
	fmt.Fprintf(w, "# HELP sgcsf_max_satellites Maximum number of satellites supported\n")
	fmt.Fprintf(w, "# TYPE sgcsf_max_satellites gauge\n")
	fmt.Fprintf(w, "sgcsf_max_satellites %d\n", gg.config.MaxSatellites)
}

// manageSatelliteConnections manages incoming satellite connections
func (gg *GroundGateway) manageSatelliteConnections(ctx context.Context) {
	// This would integrate with the QUIC transport to handle new connections
	fmt.Println("📡 Satellite connection manager started")
}

// processMessages processes incoming messages from satellites
func (gg *GroundGateway) processMessages() {
	fmt.Println("📨 Message processor started")
}

// monitorSatellites monitors satellite health and connectivity
func (gg *GroundGateway) monitorSatellites() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-gg.stopChan:
			return
		case <-ticker.C:
			gg.checkSatelliteHealth()
		}
	}
}

// checkSatelliteHealth checks health of connected satellites
func (gg *GroundGateway) checkSatelliteHealth() {
	gg.mutex.Lock()
	defer gg.mutex.Unlock()

	now := time.Now()
	for id, sat := range gg.satellites {
		if now.Sub(sat.LastSeen) > 2*time.Minute {
			log.Printf("⚠️  Satellite %s appears offline (last seen: %v)", id, sat.LastSeen)
			sat.Status = "offline"
		} else if sat.Status == "offline" {
			log.Printf("✅ Satellite %s back online", id)
			sat.Status = "online"
		}
	}
}

// sendToSatellite sends a message to a specific satellite
func (gg *GroundGateway) sendToSatellite(satelliteID string, message *types.SGCSFMessage) error {
	gg.mutex.RLock()
	sat, exists := gg.satellites[satelliteID]
	gg.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("satellite %s not connected", satelliteID)
	}

	// Queue message for satellite
	select {
	case sat.MessageQueue <- message:
		return nil
	default:
		return fmt.Errorf("satellite %s message queue full", satelliteID)
	}
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := fmt.Sscanf(value, "%d", &defaultValue); err == nil && intValue == 1 {
			return defaultValue
		}
	}
	return defaultValue
}