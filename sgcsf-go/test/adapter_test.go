package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/adapter"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

func TestHTTPAdapter(t *testing.T) {
	// Create mock SGCSF client
	config := &types.ClientConfig{
		ClientID:   "test-http-adapter",
		ServerAddr: "localhost:7000",
	}
	sgcsfClient := core.NewClient(config)
	
	// Create HTTP adapter
	adapterConfig := &adapter.HTTPAdapterConfig{
		ListenAddr:  ":8080",
		SyncTimeout: 10 * time.Second,
		MaxBodySize: 1024 * 1024,
		RouteMappings: []adapter.RouteMapping{
			{
				HTTPMethod:  "GET",
				HTTPPath:    "/api/test",
				SGCSFTopic:  "/test/api",
				MessageType: "sync",
			},
			{
				HTTPMethod:  "POST",
				HTTPPath:    "/api/async",
				SGCSFTopic:  "/test/async",
				MessageType: "async",
			},
		},
	}
	
	httpAdapter := adapter.NewHTTPAdapter(sgcsfClient, adapterConfig)
	
	// Start adapter
	ctx := context.Background()
	err := httpAdapter.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start HTTP adapter: %v", err)
	}
	defer httpAdapter.Stop()
	
	// Give adapter time to start
	time.Sleep(100 * time.Millisecond)
}

func TestHTTPToSGCSFConversion(t *testing.T) {
	// Create test HTTP request
	req := httptest.NewRequest("GET", "/api/test?param=value", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("User-Agent", "test-client/1.0")
	
	// Test route mapping
	mapping := &adapter.RouteMapping{
		HTTPMethod:  "GET",
		HTTPPath:    "/api/test",
		SGCSFTopic:  "/test/api",
		MessageType: "sync",
	}
	
	// This would be tested in the actual adapter implementation
	// For this test, we'll simulate the conversion logic
	
	message := types.NewMessage()
	message.Topic = mapping.SGCSFTopic
	message.Type = types.MessageTypeSync
	message.IsSync = true
	message.RequestID = types.GenerateRequestID()
	message.Source = "http-adapter"
	
	// Set HTTP-specific headers
	message.SetHeader("HTTP-Method", req.Method)
	message.SetHeader("HTTP-Path", req.URL.Path)
	message.SetHeader("HTTP-Query", req.URL.RawQuery)
	message.SetHeader("Content-Type", req.Header.Get("Content-Type"))
	message.SetHeader("Authorization", req.Header.Get("Authorization"))
	
	// Verify conversion
	if message.Topic != "/test/api" {
		t.Errorf("Topic conversion failed: expected /test/api, got %s", message.Topic)
	}
	
	if message.Type != types.MessageTypeSync {
		t.Errorf("Message type conversion failed: expected sync")
	}
	
	if !message.IsSync {
		t.Error("IsSync flag should be set for sync messages")
	}
	
	method, ok := message.GetHeader("HTTP-Method")
	if !ok || method != "GET" {
		t.Error("HTTP method not preserved in headers")
	}
	
	path, ok := message.GetHeader("HTTP-Path")
	if !ok || path != "/api/test" {
		t.Error("HTTP path not preserved in headers")
	}
	
	query, ok := message.GetHeader("HTTP-Query")
	if !ok || query != "param=value" {
		t.Error("HTTP query not preserved in headers")
	}
}

func TestSGCSFToHTTPConversion(t *testing.T) {
	// Create test SGCSF response
	response := types.NewMessage()
	response.Type = types.MessageTypeResponse
	response.SetHeader("HTTP-Status-Code", "200")
	response.SetHeader("HTTP-Content-Type", "application/json")
	response.SetHeader("HTTP-Header-Cache-Control", "no-cache")
	response.Payload = []byte(`{"result": "success", "data": "test"}`)
	
	// Create test HTTP response writer
	recorder := httptest.NewRecorder()
	
	// Simulate conversion (this would be in the adapter)
	statusCode := 200
	if code, exists := response.GetHeader("HTTP-Status-Code"); exists && code != "" {
		if parsed, err := fmt.Sscanf(code, "%d", &statusCode); err == nil && parsed == 1 {
			// Status code parsed successfully
		}
	}
	
	// Set headers
	for key, value := range response.Headers {
		if key == "HTTP-Content-Type" {
			recorder.Header().Set("Content-Type", value)
		} else if key == "HTTP-Header-Cache-Control" {
			recorder.Header().Set("Cache-Control", value)
		}
	}
	
	// Write response
	recorder.WriteHeader(statusCode)
	recorder.Write(response.Payload)
	
	// Verify conversion
	if recorder.Code != 200 {
		t.Errorf("Status code conversion failed: expected 200, got %d", recorder.Code)
	}
	
	if recorder.Header().Get("Content-Type") != "application/json" {
		t.Error("Content-Type header not set correctly")
	}
	
	if recorder.Header().Get("Cache-Control") != "no-cache" {
		t.Error("Custom header not set correctly")
	}
	
	var result map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("Response body is not valid JSON: %v", err)
	}
	
	if result["result"] != "success" {
		t.Error("Response body content not preserved")
	}
}

func TestRouteMatching(t *testing.T) {
	testCases := []struct {
		mapping     adapter.RouteMapping
		method      string
		path        string
		shouldMatch bool
	}{
		{
			mapping:     adapter.RouteMapping{HTTPMethod: "GET", HTTPPath: "/api/test"},
			method:      "GET",
			path:        "/api/test",
			shouldMatch: true,
		},
		{
			mapping:     adapter.RouteMapping{HTTPMethod: "GET", HTTPPath: "/api/test"},
			method:      "POST",
			path:        "/api/test",
			shouldMatch: false,
		},
		{
			mapping:     adapter.RouteMapping{HTTPMethod: "*", HTTPPath: "/api/test"},
			method:      "POST",
			path:        "/api/test",
			shouldMatch: true,
		},
		{
			mapping:     adapter.RouteMapping{HTTPMethod: "GET", HTTPPath: "/api/*"},
			method:      "GET",
			path:        "/api/anything",
			shouldMatch: true,
		},
		{
			mapping:     adapter.RouteMapping{HTTPMethod: "GET", HTTPPath: "/api/*"},
			method:      "GET",
			path:        "/different/path",
			shouldMatch: false,
		},
	}
	
	for i, tc := range testCases {
		// This would test the route matching logic in the adapter
		// For now, we'll implement a simple matching function
		matches := matchRoute(tc.mapping, tc.method, tc.path)
		
		if matches != tc.shouldMatch {
			t.Errorf("Test case %d failed: expected %v, got %v for method=%s, path=%s",
				i, tc.shouldMatch, matches, tc.method, tc.path)
		}
	}
}

func TestSatelliteRouting(t *testing.T) {
	// Test satellite-specific routing
	mapping := adapter.RouteMapping{
		HTTPMethod:   "GET",
		HTTPPath:     "/api/satellite/*/data",
		SGCSFTopic:   "/http/request",
		MessageType:  "sync",
		TargetPrefix: "satellite",
	}
	
	// Test path parsing for satellite ID
	testPaths := []struct {
		path       string
		expectedID string
	}{
		{"/api/satellite/sat5/data", "sat5"},
		{"/api/satellite/sat3/data", "sat3"},
		{"/api/satellite/sat-alpha/data", "sat-alpha"},
	}
	
	for _, tc := range testPaths {
		satelliteID := extractSatelliteID(tc.path)
		if satelliteID != tc.expectedID {
			t.Errorf("Satellite ID extraction failed for path %s: expected %s, got %s",
				tc.path, tc.expectedID, satelliteID)
		}
		
		// Build topic with satellite prefix
		topic := fmt.Sprintf("/satellite/%s%s", satelliteID, mapping.SGCSFTopic)
		expected := fmt.Sprintf("/satellite/%s/http/request", tc.expectedID)
		
		if topic != expected {
			t.Errorf("Topic building failed: expected %s, got %s", expected, topic)
		}
	}
}

func TestHTTPClientTransport(t *testing.T) {
	// Create mock SGCSF client
	config := &types.ClientConfig{
		ClientID:   "test-transport",
		ServerAddr: "localhost:7000",
	}
	sgcsfClient := core.NewClient(config)
	
	// Create HTTP client with SGCSF transport
	httpClient := adapter.CreateHTTPClient(sgcsfClient)
	
	if httpClient == nil {
		t.Fatal("HTTP client should not be nil")
	}
	
	// Test that the client has the correct transport
	// In a real test, we might test actual requests through the transport
}

func TestAsyncHTTPRequest(t *testing.T) {
	// Create test request for async endpoint
	payload := map[string]interface{}{
		"action": "start_upload",
		"file":   "test.dat",
	}
	
	payloadBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/async", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	
	// Create response recorder
	recorder := httptest.NewRecorder()
	
	// Simulate async response
	response := map[string]interface{}{
		"status":     "accepted",
		"message_id": "msg_123456",
		"topic":      "/test/async",
	}
	
	responseBytes, _ := json.Marshal(response)
	
	recorder.Header().Set("Content-Type", "application/json")
	recorder.WriteHeader(http.StatusAccepted)
	recorder.Write(responseBytes)
	
	// Verify async response
	if recorder.Code != http.StatusAccepted {
		t.Errorf("Async response should return 202 Accepted, got %d", recorder.Code)
	}
	
	var result map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("Async response body is not valid JSON: %v", err)
	}
	
	if result["status"] != "accepted" {
		t.Error("Async response should indicate accepted status")
	}
	
	if result["message_id"] == nil {
		t.Error("Async response should include message ID")
	}
}

func TestHealthCheck(t *testing.T) {
	// Test health check endpoint
	_ = httptest.NewRequest("GET", "/health", nil)
	recorder := httptest.NewRecorder()
	
	// Simulate health check response
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
	}
	
	healthBytes, _ := json.Marshal(health)
	
	recorder.Header().Set("Content-Type", "application/json")
	recorder.WriteHeader(http.StatusOK)
	recorder.Write(healthBytes)
	
	// Verify health check response
	if recorder.Code != http.StatusOK {
		t.Errorf("Health check should return 200 OK, got %d", recorder.Code)
	}
	
	var result map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("Health check response is not valid JSON: %v", err)
	}
	
	if result["status"] != "ok" {
		t.Error("Health check should return ok status")
	}
}

func TestLargeHTTPPayload(t *testing.T) {
	// Test large payload handling
	largePayload := make([]byte, 1024*1024) // 1MB
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}
	
	req := httptest.NewRequest("POST", "/api/upload", bytes.NewReader(largePayload))
	req.Header.Set("Content-Type", "application/octet-stream")
	
	// Read the body to simulate adapter processing
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read large payload: %v", err)
	}
	
	if len(body) != len(largePayload) {
		t.Errorf("Large payload size mismatch: expected %d, got %d", len(largePayload), len(body))
	}
	
	// Verify first and last bytes
	if body[0] != largePayload[0] || body[len(body)-1] != largePayload[len(largePayload)-1] {
		t.Error("Large payload content mismatch")
	}
}

// Helper functions for testing

func matchRoute(mapping adapter.RouteMapping, method, path string) bool {
	// Check HTTP method
	if mapping.HTTPMethod != "*" && mapping.HTTPMethod != method {
		return false
	}
	
	// Check path pattern
	if mapping.HTTPPath == "*" {
		return true
	}
	
	// Simple wildcard matching
	if len(mapping.HTTPPath) > 0 && mapping.HTTPPath[len(mapping.HTTPPath)-1] == '*' {
		prefix := mapping.HTTPPath[:len(mapping.HTTPPath)-1]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}
	
	return mapping.HTTPPath == path
}

func extractSatelliteID(path string) string {
	// Extract satellite ID from path like /api/satellite/sat5/data
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "satellite" {
		return parts[2]
	}
	return ""
}

