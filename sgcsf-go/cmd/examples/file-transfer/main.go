package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// FileTransferRequest represents a file transfer request
type FileTransferRequest struct {
	FileID      string `json:"file_id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	Destination string `json:"destination"`
	Checksum    string `json:"checksum"`
	Priority    string `json:"priority"`
}

// FileTransferResponse represents a file transfer response
type FileTransferResponse struct {
	Status   string `json:"status"`
	StreamID string `json:"stream_id,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

func main() {
	fmt.Println("Starting SGCSF File Transfer Example")

	// Start file server (simulates satellite file server)
	go satelliteFileServer()

	// Wait for server to start
	time.Sleep(2 * time.Second)

	// Start file transfer client
	groundFileClient()
}

func groundFileClient() {
	fmt.Println("\n🌍 Starting Ground File Transfer Client")

	// Create SGCSF client for ground station
	client := core.GroundClient("ground-data-manager", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Ground client connected to SGCSF server")

	satelliteID := "sat5"

	// 1. Download a log file
	fmt.Println("\n📥 Requesting log file download...")
	downloadLogFile(client, satelliteID, "system.log", 1024*1024) // 1MB file

	// 2. Download a large data file
	fmt.Println("\n📥 Requesting large data file download...")
	downloadLogFile(client, satelliteID, "sensor_data.csv", 10*1024*1024) // 10MB file

	// 3. Download a small configuration file
	fmt.Println("\n📥 Requesting config file download...")
	downloadLogFile(client, satelliteID, "config.json", 4*1024) // 4KB file

	fmt.Println("\n✅ File transfer client session completed")
}

func satelliteFileServer() {
	fmt.Println("\n🛰️  Starting Satellite File Server")

	// Create SGCSF client for satellite
	client := core.SatelliteClient("satellite-sat5-fileserver", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("✅ Satellite file server connected to SGCSF server")

	// Subscribe to file download requests
	subscription, err := client.Subscribe(
		"/satellite/sat5/file/download/request",
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			// Parse download request
			var req FileTransferRequest
			err := json.Unmarshal(message.Payload, &req)
			if err != nil {
				return fmt.Errorf("failed to parse download request: %v", err)
			}

			fmt.Printf("📨 Received download request for file: %s (Size: %d bytes)\n",
				req.FileName, req.FileSize)

			// Check if file exists (simulate)
			if !fileExists(req.FileName) {
				// Send error response
				errorResp := &FileTransferResponse{
					Status: "error",
					Error:  "file not found",
				}

				responseMsg := core.QuickResponse(message, errorResp)
				return client.SendResponse(message, responseMsg)
			}

			// Create stream for file transfer
			streamID := types.GenerateStreamID()
			stream, err := client.CreateStream(
				fmt.Sprintf("/satellite/sat5/file/stream/%s", streamID),
				types.StreamFile,
			)
			if err != nil {
				errorResp := &FileTransferResponse{
					Status: "error",
					Error:  fmt.Sprintf("failed to create stream: %v", err),
				}

				responseMsg := core.QuickResponse(message, errorResp)
				return client.SendResponse(message, responseMsg)
			}

			// Send success response with stream ID
			successResp := &FileTransferResponse{
				Status:   "accepted",
				StreamID: streamID,
				Message:  "file transfer initiated",
			}

			responseMsg := core.QuickResponse(message, successResp)
			err = client.SendResponse(message, responseMsg)
			if err != nil {
				stream.Close()
				return err
			}

			// Start file transfer in background
			go transferFile(stream, &req, message.Source)

			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to download requests: %v", err)
	}

	fmt.Printf("🎯 Subscribed to download requests with ID: %s\n", subscription.ID)

	// Keep server running
	select {}
}

func downloadLogFile(client core.SGCSFClient, satelliteID, fileName string, fileSize int64) {
	// Create download request
	request := &FileTransferRequest{
		FileID:      fmt.Sprintf("%s_%d", fileName, time.Now().Unix()),
		FileName:    fileName,
		FileSize:    fileSize,
		Destination: "/tmp/downloads/",
		Checksum:    fmt.Sprintf("md5_%x", rand.Int63()),
		Priority:    "normal",
	}

	// Send sync request
	message := core.NewMessage().
		Topic(fmt.Sprintf("/satellite/%s/file/download/request", satelliteID)).
		Type(types.MessageTypeSync).
		Priority(types.PriorityHigh).
		QoS(types.QoSExactlyOnce).
		ContentType("application/json").
		Source("ground-data-manager").
		Timeout(30 * time.Second).
		Payload(request).
		Build()

	fmt.Printf("📤 Sending download request for %s (%s)...\n", fileName, formatBytes(fileSize))

	response, err := client.PublishSync(message.Topic, message, 30*time.Second)
	if err != nil {
		log.Printf("❌ Download request failed: %v", err)
		return
	}

	// Parse response
	var transferResp FileTransferResponse
	err = json.Unmarshal(response.Payload, &transferResp)
	if err != nil {
		log.Printf("❌ Failed to parse download response: %v", err)
		return
	}

	if transferResp.Status == "error" {
		log.Printf("❌ Download rejected: %s", transferResp.Error)
		return
	}

	fmt.Printf("✅ Download accepted. Stream ID: %s\n", transferResp.StreamID)

	// Subscribe to stream data (in a real implementation)
	// This would handle the actual file data reception
	streamTopic := fmt.Sprintf("/satellite/%s/file/stream/%s", satelliteID, transferResp.StreamID)
	
	var subscription *types.Subscription
	subscription, err = client.Subscribe(
		streamTopic,
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			if message.IsStreamStart {
				fmt.Printf("📥 File transfer started for %s\n", fileName)
			} else if message.IsStreamEnd {
				fmt.Printf("✅ File transfer completed for %s\n", fileName)
				// Unsubscribe after completion
				go func() {
					client.Unsubscribe(subscription.ID)
				}()
			} else {
				// Handle file data chunk
				fmt.Printf("📊 Received %d bytes for %s\n", len(message.Payload), fileName)
			}
			return nil
		}),
	)

	if err != nil {
		log.Printf("❌ Failed to subscribe to stream: %v", err)
		return
	}
	
	_ = subscription // Use the subscription to avoid unused variable error

	// Wait for transfer to complete (simplified)
	time.Sleep(time.Duration(fileSize/1024/100) * time.Second) // Simulate transfer time
}

func transferFile(stream core.Stream, req *FileTransferRequest, destination string) {
	defer stream.Close()

	fmt.Printf("🚀 Starting file transfer: %s (%s) to %s\n",
		req.FileName, formatBytes(req.FileSize), destination)

	// Simulate file data
	chunkSize := 32 * 1024 // 32KB chunks
	totalBytes := req.FileSize
	transferredBytes := int64(0)

	startTime := time.Now()

	for transferredBytes < totalBytes {
		// Calculate chunk size for this iteration
		remainingBytes := totalBytes - transferredBytes
		currentChunkSize := int64(chunkSize)
		if remainingBytes < currentChunkSize {
			currentChunkSize = remainingBytes
		}

		// Generate simulated file data
		chunk := make([]byte, currentChunkSize)
		for i := range chunk {
			chunk[i] = byte(rand.Intn(256))
		}

		// Send chunk through stream
		_, err := stream.Write(chunk)
		if err != nil {
			log.Printf("❌ Failed to write chunk: %v", err)
			return
		}

		transferredBytes += currentChunkSize

		// Show progress
		progress := float64(transferredBytes) / float64(totalBytes) * 100
		if int(transferredBytes)%(1024*1024) == 0 || transferredBytes == totalBytes {
			elapsed := time.Since(startTime)
			speed := float64(transferredBytes) / elapsed.Seconds() / 1024 // KB/s
			fmt.Printf("📈 Progress: %.1f%% (%s/%s) - Speed: %.1f KB/s\n",
				progress, formatBytes(transferredBytes), formatBytes(totalBytes), speed)
		}

		// Simulate network delay
		time.Sleep(10 * time.Millisecond)
	}

	elapsed := time.Since(startTime)
	avgSpeed := float64(totalBytes) / elapsed.Seconds() / 1024 // KB/s

	fmt.Printf("✅ File transfer completed: %s\n", req.FileName)
	fmt.Printf("   Total: %s in %v (avg: %.1f KB/s)\n",
		formatBytes(totalBytes), elapsed.Round(time.Millisecond), avgSpeed)
}

func fileExists(fileName string) bool {
	// Simulate file existence check
	// In a real implementation, this would check the actual filesystem
	validFiles := []string{
		"system.log",
		"sensor_data.csv",
		"config.json",
		"telemetry.dat",
		"orbit.log",
	}

	for _, file := range validFiles {
		if file == fileName {
			return true
		}
	}

	return false
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}