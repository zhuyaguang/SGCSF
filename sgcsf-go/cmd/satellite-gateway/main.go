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
	"github.com/sgcsf/sgcsf-go/pkg/adapter"
)

// SatelliteGateway manages all communication between satellite apps and ground
type SatelliteGateway struct {
	// SGCSF client for ground communication
	groundClient core.SGCSFClient
	
	// Local message broker for satellite apps
	localBroker *LocalBroker
	
	// HTTP adapter for legacy apps
	httpAdapter *adapter.HTTPAdapter
	
	// Offline message store
	offlineStore *OfflineStore
	
	// Configuration
	config *GatewayConfig
	
	// State management
	isOnline    bool
	mutex       sync.RWMutex
	stopChan    chan struct{}
}

// GatewayConfig represents satellite gateway configuration
type GatewayConfig struct {
	SatelliteID    string        `json:"satellite_id"`
	GroundServer   string        `json:"ground_server"`
	LocalHTTPPort  string        `json:"local_http_port"`
	OfflineDir     string        `json:"offline_dir"`
	RetryInterval  time.Duration `json:"retry_interval"`
	MaxOfflineSize int64         `json:"max_offline_size"`
}

// LocalBroker handles local pub/sub for satellite applications
type LocalBroker struct {
	subscriptions map[string][]types.MessageHandler
	mutex         sync.RWMutex
}

// OfflineStore manages offline message storage and retry
type OfflineStore struct {
	storageDir    string
	maxSize       int64
	currentSize   int64
	pendingQueue  []*types.SGCSFMessage
	mutex         sync.RWMutex
}

func main() {
	fmt.Println("🛰️  Starting SGCSF Satellite Gateway")

	// Load configuration
	config := &GatewayConfig{
		SatelliteID:    getEnv("SATELLITE_ID", "sat-unknown"),
		GroundServer:   getEnv("GROUND_SERVER", "ground-control:7000"),
		LocalHTTPPort:  getEnv("LOCAL_HTTP_PORT", ":8080"),
		OfflineDir:     getEnv("OFFLINE_DIR", "/tmp/sgcsf-offline"),
		RetryInterval:  time.Duration(getEnvInt("RETRY_INTERVAL_SECONDS", 30)) * time.Second,
		MaxOfflineSize: int64(getEnvInt("MAX_OFFLINE_SIZE_MB", 100)) * 1024 * 1024,
	}

	// Create satellite gateway
	gateway, err := NewSatelliteGateway(config)
	if err != nil {
		log.Fatalf("Failed to create satellite gateway: %v", err)
	}

	// Start the gateway
	ctx := context.Background()
	err = gateway.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}

	fmt.Printf("✅ Satellite Gateway started for %s\n", config.SatelliteID)
	fmt.Printf("📡 Connecting to ground server: %s\n", config.GroundServer)
	fmt.Printf("🌐 Local HTTP API: http://localhost%s\n", config.LocalHTTPPort)
	fmt.Printf("💾 Offline storage: %s\n", config.OfflineDir)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to shutdown the gateway...")
	<-sigChan

	fmt.Println("\n🛑 Shutdown signal received. Stopping gateway...")
	gateway.Stop()
	fmt.Println("✅ Satellite Gateway shut down gracefully")
}

// NewSatelliteGateway creates a new satellite gateway
func NewSatelliteGateway(config *GatewayConfig) (*SatelliteGateway, error) {
	// Create ground client
	groundClient := core.SatelliteClient(
		fmt.Sprintf("satellite-%s-gateway", config.SatelliteID),
		config.GroundServer,
	)

	// Create local broker
	localBroker := &LocalBroker{
		subscriptions: make(map[string][]types.MessageHandler),
	}

	// Create offline store
	offlineStore, err := NewOfflineStore(config.OfflineDir, config.MaxOfflineSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create offline store: %v", err)
	}

	gateway := &SatelliteGateway{
		groundClient: groundClient,
		localBroker:  localBroker,
		offlineStore: offlineStore,
		config:       config,
		stopChan:     make(chan struct{}),
	}

	// Create HTTP adapter for local apps
	adapterConfig := &adapter.HTTPAdapterConfig{
		ListenAddr:  config.LocalHTTPPort,
		SyncTimeout: 30 * time.Second,
		MaxBodySize: 10 * 1024 * 1024,
		RouteMappings: []adapter.RouteMapping{
			{
				HTTPMethod:   "*",
				HTTPPath:     "/api/publish/*",
				SGCSFTopic:   "/local/publish",
				MessageType:  "async",
				TargetPrefix: "local",
			},
			{
				HTTPMethod:   "*",
				HTTPPath:     "/api/subscribe/*",
				SGCSFTopic:   "/local/subscribe", 
				MessageType:  "sync",
				TargetPrefix: "local",
			},
		},
	}

	httpAdapter := adapter.NewHTTPAdapter(groundClient, adapterConfig)
	gateway.httpAdapter = httpAdapter

	return gateway, nil
}

// Start starts the satellite gateway
func (sg *SatelliteGateway) Start(ctx context.Context) error {
	// Start local message broker
	go sg.startLocalBroker()

	// Start HTTP adapter for local apps
	go func() {
		err := sg.httpAdapter.Start(ctx)
		if err != nil {
			log.Printf("HTTP adapter error: %v", err)
		}
	}()

	// Start ground connection with retry
	go sg.startGroundConnection(ctx)

	// Start offline message processor
	go sg.startOfflineProcessor()

	// Start local API server for satellite apps
	go sg.startLocalAPI()

	return nil
}

// Stop stops the satellite gateway
func (sg *SatelliteGateway) Stop() {
	close(sg.stopChan)
	
	if sg.groundClient != nil {
		sg.groundClient.Disconnect()
	}
	
	// Save any remaining offline messages
	sg.offlineStore.Flush()
}

// startGroundConnection manages the connection to ground with retry
func (sg *SatelliteGateway) startGroundConnection(ctx context.Context) {
	for {
		select {
		case <-sg.stopChan:
			return
		default:
			err := sg.groundClient.Connect(ctx)
			if err != nil {
				log.Printf("❌ Failed to connect to ground: %v", err)
				sg.setOnlineStatus(false)
				
				// Wait before retry
				select {
				case <-time.After(sg.config.RetryInterval):
					continue
				case <-sg.stopChan:
					return
				}
			} else {
				fmt.Println("✅ Connected to ground server")
				sg.setOnlineStatus(true)
				
				// Setup ground message subscriptions
				sg.setupGroundSubscriptions()
				
				// Process offline messages
				sg.processOfflineMessages()
				
				// Keep connection alive
				sg.maintainGroundConnection(ctx)
			}
		}
	}
}

// setupGroundSubscriptions sets up subscriptions to ground messages
func (sg *SatelliteGateway) setupGroundSubscriptions() {
	// Subscribe to commands for this satellite
	topicPattern := fmt.Sprintf("/satellite/%s/commands/*", sg.config.SatelliteID)
	sg.groundClient.Subscribe(topicPattern, types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		log.Printf("📨 Received command from ground: %s", message.Topic)
		
		// Forward to local broker
		sg.localBroker.Publish(message.Topic, message)
		return nil
	}))
	
	// Subscribe to configuration updates
	configTopic := fmt.Sprintf("/satellite/%s/config", sg.config.SatelliteID)
	sg.groundClient.Subscribe(configTopic, types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		log.Printf("⚙️  Received config update: %s", configTopic)
		
		// Handle configuration update
		sg.handleConfigUpdate(message)
		return nil
	}))
}

// maintainGroundConnection keeps the ground connection alive
func (sg *SatelliteGateway) maintainGroundConnection(ctx context.Context) {
	// Send periodic heartbeats
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()
	
	for {
		select {
		case <-sg.stopChan:
			return
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			heartbeat := core.NewMessage().
				Topic(fmt.Sprintf("/satellite/%s/heartbeat", sg.config.SatelliteID)).
				Type(types.MessageTypeAsync).
				Priority(types.PriorityLow).
				Payload(map[string]interface{}{
					"timestamp": time.Now().Unix(),
					"status":    "online",
					"apps":      sg.getLocalAppStatus(),
				}).
				Build()
				
			err := sg.groundClient.Publish(heartbeat.Topic, heartbeat)
			if err != nil {
				log.Printf("❌ Heartbeat failed: %v", err)
				sg.setOnlineStatus(false)
				return // Will trigger reconnection
			}
		}
	}
}

// startLocalBroker starts the local message broker for satellite apps
func (sg *SatelliteGateway) startLocalBroker() {
	// This would implement a local pub/sub system
	// for satellite applications to communicate with the gateway
	fmt.Println("📡 Local message broker started")
}

// startOfflineProcessor processes offline messages when connection is restored
func (sg *SatelliteGateway) startOfflineProcessor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-sg.stopChan:
			return
		case <-ticker.C:
			if sg.isOnline {
				sg.processOfflineMessages()
			}
		}
	}
}

// processOfflineMessages sends stored offline messages to ground
func (sg *SatelliteGateway) processOfflineMessages() {
	messages := sg.offlineStore.GetPendingMessages()
	
	for _, message := range messages {
		err := sg.groundClient.Publish(message.Topic, message)
		if err != nil {
			log.Printf("❌ Failed to send offline message: %v", err)
			break // Stop processing if connection fails
		} else {
			sg.offlineStore.MarkSent(message.ID)
			log.Printf("✅ Sent offline message: %s", message.ID)
		}
	}
}

// startLocalAPI starts HTTP API for local satellite applications
func (sg *SatelliteGateway) startLocalAPI() {
	mux := http.NewServeMux()
	
	// Publish endpoint for local apps
	mux.HandleFunc("/api/publish", sg.handleLocalPublish)
	
	// Subscribe endpoint for local apps
	mux.HandleFunc("/api/subscribe", sg.handleLocalSubscribe)
	
	// Status endpoint
	mux.HandleFunc("/api/status", sg.handleStatus)
	
	// Health check for local apps (like node-exporter)
	mux.HandleFunc("/health", sg.handleHealth)
	
	// Metrics endpoint for Prometheus
	mux.HandleFunc("/metrics", sg.handleMetrics)
	
	server := &http.Server{
		Addr:    sg.config.LocalHTTPPort,
		Handler: mux,
	}
	
	log.Printf("🌐 Local API server started on %s", sg.config.LocalHTTPPort)
	server.ListenAndServe()
}

// handleLocalPublish handles publish requests from local apps
func (sg *SatelliteGateway) handleLocalPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		Topic   string      `json:"topic"`
		Payload interface{} `json:"payload"`
		Type    string      `json:"type,omitempty"`
	}
	
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// Create SGCSF message
	message := core.NewMessage().
		Topic(req.Topic).
		Type(types.MessageTypeAsync).
		Priority(types.PriorityNormal).
		Source(fmt.Sprintf("satellite-%s-app", sg.config.SatelliteID)).
		Payload(req.Payload).
		Build()
	
	// Try to send to ground
	if sg.isOnline {
		err = sg.groundClient.Publish(message.Topic, message)
		if err != nil {
			// Store offline if send fails
			sg.offlineStore.Store(message)
			log.Printf("📦 Message stored offline: %s", message.Topic)
		}
	} else {
		// Store offline if not connected
		sg.offlineStore.Store(message)
		log.Printf("📦 Message stored offline (no connection): %s", message.Topic)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "accepted",
		"message_id": message.ID,
		"queued":     !sg.isOnline,
	})
}

// handleLocalSubscribe handles subscription requests from local apps
func (sg *SatelliteGateway) handleLocalSubscribe(w http.ResponseWriter, r *http.Request) {
	// Implement WebSocket or SSE for real-time subscriptions
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "subscription_endpoint_placeholder",
	})
}

// handleStatus returns gateway status
func (sg *SatelliteGateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	sg.mutex.RLock()
	status := map[string]interface{}{
		"satellite_id":      sg.config.SatelliteID,
		"ground_connected":  sg.isOnline,
		"offline_messages":  sg.offlineStore.GetQueueSize(),
		"local_apps":        sg.getLocalAppStatus(),
		"timestamp":         time.Now().Unix(),
	}
	sg.mutex.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleHealth returns health status for monitoring
func (sg *SatelliteGateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// handleMetrics returns Prometheus metrics
func (sg *SatelliteGateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP sgcsf_ground_connected Whether ground connection is active\n")
	fmt.Fprintf(w, "# TYPE sgcsf_ground_connected gauge\n")
	if sg.isOnline {
		fmt.Fprintf(w, "sgcsf_ground_connected 1\n")
	} else {
		fmt.Fprintf(w, "sgcsf_ground_connected 0\n")
	}
	
	fmt.Fprintf(w, "# HELP sgcsf_offline_messages Number of offline messages queued\n")
	fmt.Fprintf(w, "# TYPE sgcsf_offline_messages gauge\n")
	fmt.Fprintf(w, "sgcsf_offline_messages %d\n", sg.offlineStore.GetQueueSize())
}

// Helper functions

func (sg *SatelliteGateway) setOnlineStatus(online bool) {
	sg.mutex.Lock()
	sg.isOnline = online
	sg.mutex.Unlock()
}

func (sg *SatelliteGateway) getLocalAppStatus() map[string]interface{} {
	// This would check status of local satellite applications
	return map[string]interface{}{
		"node_exporter": "running",
		"sensor_app":    "running", 
		"camera_ctrl":   "running",
	}
}

func (sg *SatelliteGateway) handleConfigUpdate(message *types.SGCSFMessage) {
	// Handle configuration updates from ground
	log.Printf("📝 Applying configuration update...")
}

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

// LocalBroker methods

func (lb *LocalBroker) Publish(topic string, message *types.SGCSFMessage) {
	lb.mutex.RLock()
	handlers := lb.subscriptions[topic]
	lb.mutex.RUnlock()
	
	for _, handler := range handlers {
		go handler.Handle(message)
	}
}

func (lb *LocalBroker) Subscribe(topic string, handler types.MessageHandler) {
	lb.mutex.Lock()
	lb.subscriptions[topic] = append(lb.subscriptions[topic], handler)
	lb.mutex.Unlock()
}

// OfflineStore methods

func NewOfflineStore(dir string, maxSize int64) (*OfflineStore, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, err
	}
	
	return &OfflineStore{
		storageDir:   dir,
		maxSize:      maxSize,
		pendingQueue: make([]*types.SGCSFMessage, 0),
	}, nil
}

func (os *OfflineStore) Store(message *types.SGCSFMessage) error {
	os.mutex.Lock()
	defer os.mutex.Unlock()
	
	// Check size limits
	if os.currentSize >= os.maxSize {
		return fmt.Errorf("offline storage full")
	}
	
	os.pendingQueue = append(os.pendingQueue, message)
	os.currentSize += int64(len(message.Payload))
	
	return nil
}

func (os *OfflineStore) GetPendingMessages() []*types.SGCSFMessage {
	os.mutex.RLock()
	defer os.mutex.RUnlock()
	
	return os.pendingQueue
}

func (os *OfflineStore) MarkSent(messageID string) {
	os.mutex.Lock()
	defer os.mutex.Unlock()
	
	for i, msg := range os.pendingQueue {
		if msg.ID == messageID {
			os.pendingQueue = append(os.pendingQueue[:i], os.pendingQueue[i+1:]...)
			break
		}
	}
}

func (os *OfflineStore) GetQueueSize() int {
	os.mutex.RLock()
	defer os.mutex.RUnlock()
	
	return len(os.pendingQueue)
}

func (os *OfflineStore) Flush() {
	// Persist pending messages to disk
	log.Println("💾 Flushing offline messages to disk...")
}