package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/adapter"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

func main() {
	fmt.Println("Starting SGCSF HTTP Adapter Demo")

	// Start satellite HTTP server (simulates existing HTTP service on satellite)
	go satelliteHTTPServer()

	// Start ground HTTP adapter
	go groundHTTPAdapter()

	// Wait for services to start
	time.Sleep(3 * time.Second)

	// Start HTTP client demo
	httpClientDemo()
}

func satelliteHTTPServer() {
	fmt.Println("\n🛰️  Starting Satellite HTTP Server (simulates existing service)")

	// Create SGCSF client for satellite HTTP server
	client := core.SatelliteClient("satellite-sat5-httpserver", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Satellite HTTP server connected to SGCSF")

	// Subscribe to HTTP requests
	subscription, err := client.Subscribe(
		"/satellite/sat5/http/request",
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			// Parse HTTP request from SGCSF message
			method, _ := message.GetHeader("HTTP-Method")
			path, _ := message.GetHeader("HTTP-Path")
			query, _ := message.GetHeader("HTTP-Query")

			fmt.Printf("📨 Received HTTP request: %s %s", method, path)
			if query != "" {
				fmt.Printf("?%s", query)
			}
			fmt.Println()

			// Process the request and generate response
			response := processHTTPRequest(method, path, query, message.Payload)

			// Send HTTP response back through SGCSF
			responseMsg := core.QuickResponse(message, response.Body)
			responseMsg.SetHeader("HTTP-Status-Code", fmt.Sprintf("%d", response.StatusCode))
			responseMsg.SetHeader("HTTP-Content-Type", response.ContentType)

			for key, value := range response.Headers {
				responseMsg.SetHeader("HTTP-Header-"+key, value)
			}

			err := client.SendResponse(message, responseMsg)
			if err != nil {
				return fmt.Errorf("failed to send HTTP response: %v", err)
			}

			fmt.Printf("✅ Sent HTTP response: %d %s\n", response.StatusCode, response.StatusText)
			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to HTTP requests: %v", err)
	}

	fmt.Printf("🎯 Subscribed to HTTP requests with ID: %s\n", subscription.ID)

	// Keep server running
	select {}
}

func groundHTTPAdapter() {
	fmt.Println("\n🌍 Starting Ground HTTP Adapter")

	// Create SGCSF client for HTTP adapter
	sgcsfClient := core.GroundClient("ground-http-adapter", "localhost:7000")

	// Create HTTP adapter configuration
	adapterConfig := &adapter.HTTPAdapterConfig{
		ListenAddr:  ":8080",
		SyncTimeout: 30 * time.Second,
		MaxBodySize: 10 * 1024 * 1024, // 10MB
		RouteMappings: []adapter.RouteMapping{
			// Satellite API routes
			{
				HTTPMethod:   "GET",
				HTTPPath:     "/api/satellite/*/status",
				SGCSFTopic:   "/http/request",
				MessageType:  "sync",
				TargetPrefix: "satellite",
			},
			{
				HTTPMethod:   "GET",
				HTTPPath:     "/api/satellite/*/data",
				SGCSFTopic:   "/http/request",
				MessageType:  "sync",
				TargetPrefix: "satellite",
			},
			{
				HTTPMethod:   "POST",
				HTTPPath:     "/api/satellite/*/command",
				SGCSFTopic:   "/http/request",
				MessageType:  "sync",
				TargetPrefix: "satellite",
			},
			// General HTTP proxy
			{
				HTTPMethod:  "*",
				HTTPPath:    "/api/satellite/*",
				SGCSFTopic:  "/http/request",
				MessageType: "sync",
				TargetPrefix: "satellite",
			},
		},
	}

	httpAdapter := adapter.NewHTTPAdapter(sgcsfClient, adapterConfig)

	// Start the adapter
	ctx := context.Background()
	err := httpAdapter.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start HTTP adapter: %v", err)
	}

	fmt.Println("✅ HTTP adapter started on :8080")

	// Keep adapter running
	select {}
}

func httpClientDemo() {
	fmt.Println("\n💻 Starting HTTP Client Demo")

	// Wait for adapter to be ready
	time.Sleep(1 * time.Second)

	baseURL := "http://localhost:8080"

	// 1. Get satellite status
	fmt.Println("\n📡 Getting satellite status...")
	statusResp, err := http.Get(baseURL + "/api/satellite/sat5/status")
	if err != nil {
		log.Printf("❌ Status request failed: %v", err)
	} else {
		handleHTTPResponse("Satellite Status", statusResp)
	}

	// 2. Get sensor data
	fmt.Println("\n📊 Getting sensor data...")
	dataResp, err := http.Get(baseURL + "/api/satellite/sat5/data?type=sensors&interval=60")
	if err != nil {
		log.Printf("❌ Data request failed: %v", err)
	} else {
		handleHTTPResponse("Sensor Data", dataResp)
	}

	// 3. Send command
	fmt.Println("\n🎮 Sending command...")
	commandData := map[string]interface{}{
		"command": "start_experiment",
		"params": map[string]interface{}{
			"experiment_id": "exp_001",
			"duration":      300,
		},
	}

	commandJSON, _ := json.Marshal(commandData)
	commandResp, err := http.Post(
		baseURL+"/api/satellite/sat5/command",
		"application/json",
		bytes.NewReader(commandJSON),
	)
	if err != nil {
		log.Printf("❌ Command request failed: %v", err)
	} else {
		handleHTTPResponse("Command Execution", commandResp)
	}

	// 4. Invalid endpoint (to test error handling)
	fmt.Println("\n❌ Testing invalid endpoint...")
	invalidResp, err := http.Get(baseURL + "/api/satellite/sat5/invalid")
	if err != nil {
		log.Printf("❌ Invalid request failed as expected: %v", err)
	} else {
		handleHTTPResponse("Invalid Endpoint", invalidResp)
	}

	fmt.Println("\n✅ HTTP client demo completed")
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode  int
	StatusText  string
	ContentType string
	Headers     map[string]string
	Body        []byte
}

func processHTTPRequest(method, path, query string, body []byte) *HTTPResponse {
	// Simulate processing delay
	time.Sleep(100 * time.Millisecond)

	response := &HTTPResponse{
		Headers: make(map[string]string),
	}

	// Add common headers
	response.Headers["Server"] = "SGCSF-Satellite/1.0"
	response.Headers["X-Satellite-ID"] = "sat5"

	// Route based on path
	switch {
	case path == "/api/status":
		response.StatusCode = 200
		response.StatusText = "OK"
		response.ContentType = "application/json"
		
		statusData := map[string]interface{}{
			"satellite_id":   "sat5",
			"status":         "operational",
			"power_level":    87.3,
			"temperature":    23.8,
			"altitude_km":    408.2,
			"velocity_kmh":   27580,
			"communication":  "nominal",
			"systems": map[string]string{
				"power":        "nominal",
				"thermal":      "nominal",
				"communication": "nominal",
				"navigation":   "nominal",
			},
			"timestamp": time.Now().Format(time.RFC3339),
		}
		
		response.Body, _ = json.MarshalIndent(statusData, "", "  ")

	case path == "/api/data":
		response.StatusCode = 200
		response.StatusText = "OK"
		response.ContentType = "application/json"
		
		sensorData := map[string]interface{}{
			"satellite_id": "sat5",
			"data_type":    "sensors",
			"timestamp":    time.Now().Format(time.RFC3339),
			"sensors": []map[string]interface{}{
				{
					"id":          "temp_001",
					"type":        "temperature",
					"value":       23.8,
					"unit":        "celsius",
					"status":      "nominal",
				},
				{
					"id":          "humid_001",
					"type":        "humidity",
					"value":       45.2,
					"unit":        "percent",
					"status":      "nominal",
				},
				{
					"id":          "press_001",
					"type":        "pressure",
					"value":       1013.25,
					"unit":        "hPa",
					"status":      "nominal",
				},
			},
		}
		
		response.Body, _ = json.MarshalIndent(sensorData, "", "  ")

	case path == "/api/command" && method == "POST":
		response.StatusCode = 200
		response.StatusText = "OK"
		response.ContentType = "application/json"
		
		// Parse command from body
		var command map[string]interface{}
		json.Unmarshal(body, &command)
		
		commandResponse := map[string]interface{}{
			"status":      "accepted",
			"command":     command["command"],
			"execution_id": fmt.Sprintf("exec_%d", time.Now().Unix()),
			"estimated_duration": "300 seconds",
			"message":     "Command queued for execution",
			"timestamp":   time.Now().Format(time.RFC3339),
		}
		
		response.Body, _ = json.MarshalIndent(commandResponse, "", "  ")

	default:
		response.StatusCode = 404
		response.StatusText = "Not Found"
		response.ContentType = "application/json"
		
		errorData := map[string]interface{}{
			"error":     "endpoint not found",
			"path":      path,
			"method":    method,
			"message":   "The requested endpoint is not available on this satellite",
			"timestamp": time.Now().Format(time.RFC3339),
		}
		
		response.Body, _ = json.MarshalIndent(errorData, "", "  ")
	}

	return response
}

func handleHTTPResponse(operation string, resp *http.Response) {
	defer resp.Body.Close()

	fmt.Printf("📋 %s Response:\n", operation)
	fmt.Printf("   Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Printf("   Content-Type: %s\n", resp.Header.Get("Content-Type"))

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("   Error reading body: %v\n", err)
		return
	}

	// Pretty print JSON response
	if resp.Header.Get("Content-Type") == "application/json" {
		var jsonData interface{}
		if err := json.Unmarshal(body, &jsonData); err == nil {
			prettyJSON, _ := json.MarshalIndent(jsonData, "   ", "  ")
			fmt.Printf("   Body:\n   %s\n", string(prettyJSON))
		} else {
			fmt.Printf("   Body: %s\n", string(body))
		}
	} else {
		fmt.Printf("   Body: %s\n", string(body))
	}

	// Show relevant headers
	if serverHeader := resp.Header.Get("Server"); serverHeader != "" {
		fmt.Printf("   Server: %s\n", serverHeader)
	}
	if satelliteID := resp.Header.Get("X-Satellite-ID"); satelliteID != "" {
		fmt.Printf("   Satellite-ID: %s\n", satelliteID)
	}

	fmt.Println()
}