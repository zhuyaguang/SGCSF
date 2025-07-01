package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// CommandRequest represents a command sent to satellite
type CommandRequest struct {
	Command   string                 `json:"command"`
	Target    string                 `json:"target"`
	Params    map[string]interface{} `json:"params,omitempty"`
	RequestID string                 `json:"request_id"`
	Timestamp int64                  `json:"timestamp"`
}

// CommandResponse represents a response from satellite
type CommandResponse struct {
	Status    string                 `json:"status"`
	Result    map[string]interface{} `json:"result,omitempty"`
	Error     string                 `json:"error,omitempty"`
	RequestID string                 `json:"request_id"`
	Timestamp int64                  `json:"timestamp"`
}

func main() {
	fmt.Println("Starting SGCSF Synchronous Communication Example")

	// Start satellite command handler (simulates satellite)
	go satelliteCommandHandler()

	// Wait a moment for satellite to start
	time.Sleep(2 * time.Second)

	// Start ground control client
	groundControlClient()
}

func groundControlClient() {
	fmt.Println("\n🌍 Starting Ground Control Client")

	// Create SGCSF client for ground control
	client := core.GroundClient("ground-control-center", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Ground control connected to SGCSF server")

	// Execute various commands
	satelliteID := "sat5"

	// 1. Get satellite status
	fmt.Println("\n📡 Sending status request to satellite...")
	statusCmd := &CommandRequest{
		Command:   "get_status",
		Target:    satelliteID,
		RequestID: types.GenerateRequestID(),
		Timestamp: time.Now().Unix(),
	}

	response := sendSyncCommand(client, satelliteID, statusCmd)
	printCommandResult("Status Request", statusCmd, response)

	// 2. Start data collection
	fmt.Println("\n📊 Starting data collection...")
	collectCmd := &CommandRequest{
		Command: "start_data_collection",
		Target:  satelliteID,
		Params: map[string]interface{}{
			"sensors":  []string{"temperature", "humidity", "pressure"},
			"interval": 30,
			"duration": 300,
		},
		RequestID: types.GenerateRequestID(),
		Timestamp: time.Now().Unix(),
	}

	response = sendSyncCommand(client, satelliteID, collectCmd)
	printCommandResult("Data Collection", collectCmd, response)

	// 3. Download file
	fmt.Println("\n📥 Requesting file download...")
	downloadCmd := &CommandRequest{
		Command: "download_file",
		Target:  satelliteID,
		Params: map[string]interface{}{
			"file_path":   "/data/logs/system.log",
			"destination": "/tmp/downloads/",
			"checksum":    true,
		},
		RequestID: types.GenerateRequestID(),
		Timestamp: time.Now().Unix(),
	}

	response = sendSyncCommand(client, satelliteID, downloadCmd)
	printCommandResult("File Download", downloadCmd, response)

	// 4. Invalid command (to test error handling)
	fmt.Println("\n❌ Sending invalid command...")
	invalidCmd := &CommandRequest{
		Command:   "invalid_command",
		Target:    satelliteID,
		RequestID: types.GenerateRequestID(),
		Timestamp: time.Now().Unix(),
	}

	response = sendSyncCommand(client, satelliteID, invalidCmd)
	printCommandResult("Invalid Command", invalidCmd, response)

	fmt.Println("\n✅ Ground control session completed")
}

func satelliteCommandHandler() {
	fmt.Println("\n🛰️  Starting Satellite Command Handler")

	// Create SGCSF client for satellite
	client := core.SatelliteClient("satellite-sat5-commander", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Satellite connected to SGCSF server")

	// Subscribe to command requests
	subscription, err := client.Subscribe(
		"/satellite/sat5/commands/request",
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			// Parse command request
			var cmdReq CommandRequest
			err := json.Unmarshal(message.Payload, &cmdReq)
			if err != nil {
				return fmt.Errorf("failed to parse command request: %v", err)
			}

			fmt.Printf("📨 Received command: %s (ID: %s)\n", cmdReq.Command, cmdReq.RequestID)

			// Process command
			response := processCommand(&cmdReq)

			// Send response
			responseMsg := core.QuickResponse(message, response)
			responseMsg.ContentType = "application/json"

			err = client.SendResponse(message, responseMsg)
			if err != nil {
				return fmt.Errorf("failed to send response: %v", err)
			}

			fmt.Printf("✅ Sent response for command: %s\n", cmdReq.Command)
			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to commands: %v", err)
	}

	fmt.Printf("🎯 Subscribed to commands with ID: %s\n", subscription.ID)

	// Keep satellite running
	select {}
}

func sendSyncCommand(client core.SGCSFClient, satelliteID string, cmd *CommandRequest) *CommandResponse {
	// Create sync message
	message := core.NewMessage().
		Topic(fmt.Sprintf("/satellite/%s/commands/request", satelliteID)).
		Type(types.MessageTypeSync).
		Priority(types.PriorityHigh).
		QoS(types.QoSExactlyOnce).
		ContentType("application/json").
		Source("ground-control-center").
		Destination(fmt.Sprintf("satellite-%s-commander", satelliteID)).
		Timeout(30 * time.Second).
		TTL(5 * time.Minute).
		Payload(cmd).
		Build()

	// Send sync request
	response, err := client.PublishSync(message.Topic, message, 30*time.Second)
	if err != nil {
		log.Printf("❌ Command failed: %v", err)
		return &CommandResponse{
			Status:    "error",
			Error:     err.Error(),
			RequestID: cmd.RequestID,
			Timestamp: time.Now().Unix(),
		}
	}

	// Parse response
	var cmdResp CommandResponse
	err = json.Unmarshal(response.Payload, &cmdResp)
	if err != nil {
		log.Printf("❌ Failed to parse response: %v", err)
		return &CommandResponse{
			Status:    "error",
			Error:     "invalid response format",
			RequestID: cmd.RequestID,
			Timestamp: time.Now().Unix(),
		}
	}

	return &cmdResp
}

func processCommand(cmd *CommandRequest) *CommandResponse {
	// Simulate command processing delay
	time.Sleep(1 * time.Second)

	response := &CommandResponse{
		RequestID: cmd.RequestID,
		Timestamp: time.Now().Unix(),
	}

	switch cmd.Command {
	case "get_status":
		response.Status = "success"
		response.Result = map[string]interface{}{
			"satellite_id":    "sat5",
			"power_level":     85.7,
			"temperature":     23.4,
			"communication":   "nominal",
			"orbit_altitude":  408.5,
			"system_health":   "good",
			"uptime_seconds":  3600 * 24 * 15, // 15 days
		}

	case "start_data_collection":
		response.Status = "success"
		response.Result = map[string]interface{}{
			"collection_id":   "col_" + types.GenerateRequestID(),
			"sensors_active":  cmd.Params["sensors"],
			"interval":        cmd.Params["interval"],
			"duration":        cmd.Params["duration"],
			"estimated_data":  "~150 KB",
			"start_time":      time.Now().Format(time.RFC3339),
		}

	case "download_file":
		response.Status = "success"
		response.Result = map[string]interface{}{
			"download_id":     "dl_" + types.GenerateRequestID(),
			"file_path":       cmd.Params["file_path"],
			"file_size":       "2.3 MB",
			"estimated_time":  "45 seconds",
			"stream_id":       "stream_" + types.GenerateRequestID(),
			"checksum_md5":    "a1b2c3d4e5f6789",
		}

	default:
		response.Status = "error"
		response.Error = fmt.Sprintf("unknown command: %s", cmd.Command)
	}

	return response
}

func printCommandResult(operation string, cmd *CommandRequest, resp *CommandResponse) {
	fmt.Printf("📋 %s Result:\n", operation)
	fmt.Printf("   Command: %s\n", cmd.Command)
	fmt.Printf("   Request ID: %s\n", cmd.RequestID)
	fmt.Printf("   Status: %s\n", resp.Status)

	if resp.Status == "success" && resp.Result != nil {
		fmt.Println("   Result:")
		for key, value := range resp.Result {
			fmt.Printf("     %s: %v\n", key, value)
		}
	}

	if resp.Error != "" {
		fmt.Printf("   Error: %s\n", resp.Error)
	}

	fmt.Printf("   Response Time: %d\n", resp.Timestamp)
	fmt.Println()
}