package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// HTTPAdapter adapts HTTP requests to SGCSF messages
type HTTPAdapter struct {
	sgcsfClient     core.SGCSFClient
	httpServer      *http.Server
	routeMappings   []RouteMapping
	syncTimeout     time.Duration
	serverMux       *http.ServeMux
	running         bool
	mutex           sync.RWMutex
}

// RouteMapping defines HTTP route to SGCSF topic mapping
type RouteMapping struct {
	HTTPMethod   string `json:"http_method"`   // GET/POST/PUT/DELETE
	HTTPPath     string `json:"http_path"`     // /api/v1/pods
	SGCSFTopic   string `json:"sgcsf_topic"`   // /k8s/pods/operations
	MessageType  string `json:"message_type"`  // sync/async
	TargetPrefix string `json:"target_prefix"` // for satellite routing
}

// HTTPAdapterConfig represents HTTP adapter configuration
type HTTPAdapterConfig struct {
	ListenAddr    string         `json:"listen_addr"`
	RouteMappings []RouteMapping `json:"route_mappings"`
	SyncTimeout   time.Duration  `json:"sync_timeout"`
	MaxBodySize   int64          `json:"max_body_size"`
}

// NewHTTPAdapter creates a new HTTP adapter
func NewHTTPAdapter(sgcsfClient core.SGCSFClient, config *HTTPAdapterConfig) *HTTPAdapter {
	adapter := &HTTPAdapter{
		sgcsfClient:   sgcsfClient,
		routeMappings: config.RouteMappings,
		syncTimeout:   config.SyncTimeout,
		serverMux:     http.NewServeMux(),
	}

	if adapter.syncTimeout == 0 {
		adapter.syncTimeout = 30 * time.Second
	}

	// Setup default route mappings if none provided
	if len(adapter.routeMappings) == 0 {
		adapter.routeMappings = getDefaultRouteMappings()
	}

	// Setup HTTP server
	adapter.httpServer = &http.Server{
		Addr:    config.ListenAddr,
		Handler: adapter.serverMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Register route handlers
	adapter.setupRoutes()

	return adapter
}

// Start starts the HTTP adapter
func (h *HTTPAdapter) Start(ctx context.Context) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.running {
		return fmt.Errorf("HTTP adapter already running")
	}

	// Ensure SGCSF client is connected
	if !h.sgcsfClient.IsConnected() {
		err := h.sgcsfClient.Connect(ctx)
		if err != nil {
			return fmt.Errorf("failed to connect SGCSF client: %v", err)
		}
	}

	h.running = true

	// Start HTTP server in background
	go func() {
		fmt.Printf("HTTP Adapter listening on %s\n", h.httpServer.Addr)
		if err := h.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the HTTP adapter
func (h *HTTPAdapter) Stop() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if !h.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := h.httpServer.Shutdown(ctx)
	h.running = false
	return err
}

// setupRoutes sets up HTTP route handlers
func (h *HTTPAdapter) setupRoutes() {
	// Generic handler for all routes
	h.serverMux.HandleFunc("/", h.handleHTTPRequest)
	
	// Health check endpoint
	h.serverMux.HandleFunc("/health", h.handleHealthCheck)
}

// handleHTTPRequest handles incoming HTTP requests
func (h *HTTPAdapter) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Skip health check
	if r.URL.Path == "/health" {
		return
	}

	// Find matching route mapping
	mapping := h.findRouteMapping(r.Method, r.URL.Path)
	if mapping == nil {
		http.Error(w, "No SGCSF mapping found for this route", http.StatusNotFound)
		return
	}

	// Convert HTTP request to SGCSF message
	sgcsfMsg, err := h.convertHTTPToSGCSF(r, mapping)
	if err != nil {
		http.Error(w, fmt.Sprintf("Request conversion failed: %v", err), http.StatusBadRequest)
		return
	}

	// Handle based on message type
	if mapping.MessageType == "sync" {
		h.handleSyncRequest(w, r, sgcsfMsg, mapping)
	} else {
		h.handleAsyncRequest(w, r, sgcsfMsg, mapping)
	}
}

// handleSyncRequest handles synchronous HTTP requests
func (h *HTTPAdapter) handleSyncRequest(w http.ResponseWriter, r *http.Request, sgcsfMsg *types.SGCSFMessage, mapping *RouteMapping) {
	// Send sync message and wait for response
	response, err := h.sgcsfClient.PublishSync(sgcsfMsg.Topic, sgcsfMsg, h.syncTimeout)
	if err != nil {
		http.Error(w, fmt.Sprintf("SGCSF sync request failed: %v", err), http.StatusBadGateway)
		return
	}

	// Convert SGCSF response back to HTTP
	h.convertSGCSFResponseToHTTP(response, w)
}

// handleAsyncRequest handles asynchronous HTTP requests
func (h *HTTPAdapter) handleAsyncRequest(w http.ResponseWriter, r *http.Request, sgcsfMsg *types.SGCSFMessage, mapping *RouteMapping) {
	// Send async message
	err := h.sgcsfClient.Publish(sgcsfMsg.Topic, sgcsfMsg)
	if err != nil {
		http.Error(w, fmt.Sprintf("SGCSF publish failed: %v", err), http.StatusBadGateway)
		return
	}

	// Return immediate response for async requests
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "accepted",
		"message_id": sgcsfMsg.ID,
		"topic":      sgcsfMsg.Topic,
	})
}

// handleHealthCheck handles health check requests
func (h *HTTPAdapter) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK

	if !h.sgcsfClient.IsConnected() {
		status = "sgcsf_disconnected"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"status": status,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// convertHTTPToSGCSF converts HTTP request to SGCSF message
func (h *HTTPAdapter) convertHTTPToSGCSF(r *http.Request, mapping *RouteMapping) (*types.SGCSFMessage, error) {
	// Create SGCSF message
	message := types.NewMessage()
	message.Topic = h.buildTopic(mapping, r)
	message.Source = "http-adapter"
	
	// Set message type
	if mapping.MessageType == "sync" {
		message.Type = types.MessageTypeSync
		message.IsSync = true
		message.RequestID = types.GenerateRequestID()
	} else {
		message.Type = types.MessageTypeAsync
	}

	// Set HTTP-specific headers
	message.SetHeader("HTTP-Method", r.Method)
	message.SetHeader("HTTP-Path", r.URL.Path)
	message.SetHeader("HTTP-Query", r.URL.RawQuery)
	message.SetHeader("Content-Type", r.Header.Get("Content-Type"))
	message.SetHeader("User-Agent", r.Header.Get("User-Agent"))
	message.SetHeader("Authorization", r.Header.Get("Authorization"))

	// Copy other important headers
	for name, values := range r.Header {
		if len(values) > 0 && h.shouldCopyHeader(name) {
			message.SetHeader("HTTP-Header-"+name, values[0])
		}
	}

	// Read request body if present
	if r.ContentLength > 0 {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %v", err)
		}
		message.Payload = body
	}

	return message, nil
}

// convertSGCSFResponseToHTTP converts SGCSF response to HTTP response
func (h *HTTPAdapter) convertSGCSFResponseToHTTP(sgcsfResponse *types.SGCSFMessage, w http.ResponseWriter) {
	// Set HTTP status code
	statusCode := http.StatusOK
	if code, exists := sgcsfResponse.GetHeader("HTTP-Status-Code"); exists {
		if parsed, err := strconv.Atoi(code); err == nil {
			statusCode = parsed
		}
	}

	// Set response headers
	for key, value := range sgcsfResponse.Headers {
		if strings.HasPrefix(key, "HTTP-Header-") {
			headerName := strings.TrimPrefix(key, "HTTP-Header-")
			w.Header().Set(headerName, value)
		}
	}

	// Set Content-Type
	if contentType, exists := sgcsfResponse.GetHeader("HTTP-Content-Type"); exists {
		w.Header().Set("Content-Type", contentType)
	} else if len(sgcsfResponse.Payload) > 0 {
		// Try to detect content type
		w.Header().Set("Content-Type", http.DetectContentType(sgcsfResponse.Payload))
	}

	// Write status code and response body
	w.WriteHeader(statusCode)
	if len(sgcsfResponse.Payload) > 0 {
		w.Write(sgcsfResponse.Payload)
	}
}

// findRouteMapping finds the appropriate route mapping
func (h *HTTPAdapter) findRouteMapping(method, path string) *RouteMapping {
	for _, mapping := range h.routeMappings {
		if h.matchRoute(mapping, method, path) {
			return &mapping
		}
	}
	return nil
}

// matchRoute checks if a route mapping matches the request
func (h *HTTPAdapter) matchRoute(mapping RouteMapping, method, path string) bool {
	// Check HTTP method
	if mapping.HTTPMethod != "*" && mapping.HTTPMethod != method {
		return false
	}

	// Check path pattern
	if mapping.HTTPPath == "*" {
		return true
	}

	// Simple wildcard matching
	if strings.HasSuffix(mapping.HTTPPath, "*") {
		prefix := strings.TrimSuffix(mapping.HTTPPath, "*")
		return strings.HasPrefix(path, prefix)
	}

	return mapping.HTTPPath == path
}

// buildTopic builds the SGCSF topic from mapping and request
func (h *HTTPAdapter) buildTopic(mapping *RouteMapping, r *http.Request) string {
	topic := mapping.SGCSFTopic

	// Handle satellite routing
	if mapping.TargetPrefix != "" {
		// Extract satellite ID from path
		// e.g., /api/satellite/sat5/data -> sat5
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathParts) >= 3 && pathParts[1] == "satellite" {
			satelliteID := pathParts[2]
			topic = fmt.Sprintf("/%s/%s%s", mapping.TargetPrefix, satelliteID, topic)
		}
	}

	return topic
}

// shouldCopyHeader determines if an HTTP header should be copied
func (h *HTTPAdapter) shouldCopyHeader(name string) bool {
	lowerName := strings.ToLower(name)
	skipHeaders := []string{
		"connection", "upgrade", "proxy-connection",
		"proxy-authenticate", "proxy-authorization",
		"te", "trailers", "transfer-encoding",
	}

	for _, skip := range skipHeaders {
		if lowerName == skip {
			return false
		}
	}

	return true
}

// getDefaultRouteMappings returns default route mappings
func getDefaultRouteMappings() []RouteMapping {
	return []RouteMapping{
		// Kubernetes API mappings
		{HTTPMethod: "GET", HTTPPath: "/api/v1/pods", SGCSFTopic: "/k8s/pods/query", MessageType: "sync"},
		{HTTPMethod: "POST", HTTPPath: "/api/v1/pods", SGCSFTopic: "/k8s/pods/create", MessageType: "sync"},
		{HTTPMethod: "PUT", HTTPPath: "/api/v1/pods/*", SGCSFTopic: "/k8s/pods/update", MessageType: "sync"},
		{HTTPMethod: "DELETE", HTTPPath: "/api/v1/pods/*", SGCSFTopic: "/k8s/pods/delete", MessageType: "sync"},

		// Prometheus metrics
		{HTTPMethod: "GET", HTTPPath: "/metrics", SGCSFTopic: "/monitoring/metrics/collect", MessageType: "sync"},

		// Data operations
		{HTTPMethod: "POST", HTTPPath: "/upload/*", SGCSFTopic: "/data/upload/start", MessageType: "async"},
		{HTTPMethod: "GET", HTTPPath: "/download/*", SGCSFTopic: "/data/download/start", MessageType: "async"},

		// Satellite-specific routes
		{HTTPMethod: "GET", HTTPPath: "/api/satellite/*/data", SGCSFTopic: "/http/request", MessageType: "sync", TargetPrefix: "satellite"},
		{HTTPMethod: "*", HTTPPath: "/api/satellite/*", SGCSFTopic: "/http/request", MessageType: "sync", TargetPrefix: "satellite"},
	}
}

// CreateHTTPClient creates an HTTP client that uses SGCSF for satellite communication
func CreateHTTPClient(sgcsfClient core.SGCSFClient) *http.Client {
	return &http.Client{
		Transport: &SGCSFTransport{
			sgcsfClient: sgcsfClient,
			httpTransport: http.DefaultTransport,
		},
		Timeout: 30 * time.Second,
	}
}

// SGCSFTransport implements http.RoundTripper for SGCSF communication
type SGCSFTransport struct {
	sgcsfClient   core.SGCSFClient
	httpTransport http.RoundTripper
}

// RoundTrip implements http.RoundTripper interface
func (t *SGCSFTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if this should use SGCSF (satellite URLs)
	if strings.Contains(req.URL.Host, "satellite-") || strings.Contains(req.URL.Path, "/satellite/") {
		return t.roundTripSGCSF(req)
	}

	// Use normal HTTP transport for other requests
	return t.httpTransport.RoundTrip(req)
}

// roundTripSGCSF handles requests through SGCSF
func (t *SGCSFTransport) roundTripSGCSF(req *http.Request) (*http.Response, error) {
	// Convert HTTP request to SGCSF message
	message := types.NewMessage()
	message.Topic = "/http/request"
	message.Type = types.MessageTypeSync
	message.IsSync = true
	message.RequestID = types.GenerateRequestID()

	// Set HTTP details in headers
	message.SetHeader("HTTP-Method", req.Method)
	message.SetHeader("HTTP-Path", req.URL.Path)
	message.SetHeader("HTTP-Query", req.URL.RawQuery)
	message.SetHeader("HTTP-Host", req.URL.Host)

	// Copy headers
	for name, values := range req.Header {
		if len(values) > 0 {
			message.SetHeader("HTTP-Header-"+name, values[0])
		}
	}

	// Read body
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		message.Payload = body
	}

	// Send through SGCSF
	response, err := t.sgcsfClient.PublishSync(message.Topic, message, 30*time.Second)
	if err != nil {
		return nil, err
	}

	// Convert SGCSF response to HTTP response
	return t.convertToHTTPResponse(response)
}

// convertToHTTPResponse converts SGCSF response to HTTP response
func (t *SGCSFTransport) convertToHTTPResponse(sgcsfResp *types.SGCSFMessage) (*http.Response, error) {
	// Create HTTP response
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(sgcsfResp.Payload)),
	}

	// Set status code
	if code, exists := sgcsfResp.GetHeader("HTTP-Status-Code"); exists {
		if parsed, err := strconv.Atoi(code); err == nil {
			resp.StatusCode = parsed
		}
	}

	// Set headers
	for key, value := range sgcsfResp.Headers {
		if strings.HasPrefix(key, "HTTP-Header-") {
			headerName := strings.TrimPrefix(key, "HTTP-Header-")
			resp.Header.Set(headerName, value)
		}
	}

	return resp, nil
}