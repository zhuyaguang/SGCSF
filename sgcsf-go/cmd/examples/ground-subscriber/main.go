package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

// SensorData represents satellite sensor data
type SensorData struct {
	SensorID    string  `json:"sensor_id"`
	Temperature float64 `json:"temperature"`
	Humidity    float64 `json:"humidity"`
	Pressure    float64 `json:"pressure"`
	Timestamp   int64   `json:"timestamp"`
	SatelliteID string  `json:"satellite_id"`
}

func main() {
	fmt.Println("Starting SGCSF Ground Station Data Subscriber Example")

	// Create SGCSF client for ground station
	client := core.GroundClient("ground-monitoring-station", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("Connected to SGCSF server")

	// Subscribe to sensor data from all satellites
	sensorSubscription, err := client.Subscribe(
		"/satellite/+/sensors/temperature", // Wildcard to receive from all satellites
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			// Parse sensor data
			var data SensorData
			err := json.Unmarshal(message.Payload, &data)
			if err != nil {
				return fmt.Errorf("failed to parse sensor data: %v", err)
			}

			// Process sensor data
			fmt.Printf("📊 Sensor Data from %s (%s): %.1f°C, %.1f%%, %.1f hPa [%s]\n",
				data.SensorID, data.SatelliteID, data.Temperature, data.Humidity, data.Pressure,
				message.Topic)

			// Store data (in real application, this would go to a database)
			storeData := map[string]interface{}{
				"source":      message.Source,
				"received_at": message.Timestamp,
				"data":        data,
				"topic":       message.Topic,
			}

			// Simulate data storage
			dataJSON, _ := json.MarshalIndent(storeData, "", "  ")
			fmt.Printf("💾 Stored: %s\n", string(dataJSON))

			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to sensor data: %v", err)
	}

	fmt.Printf("📡 Subscribed to sensor data with ID: %s\n", sensorSubscription.ID)

	// Subscribe to temperature alerts
	alertSubscription, err := client.Subscribe(
		"/satellite/+/alerts/temperature", // Temperature alerts from all satellites
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			// Parse alert data
			var alertData map[string]interface{}
			err := json.Unmarshal(message.Payload, &alertData)
			if err != nil {
				return fmt.Errorf("failed to parse alert data: %v", err)
			}

			// Handle critical alert
			fmt.Printf("🚨 CRITICAL ALERT: %s\n", alertData["alert"])
			fmt.Printf("   Temperature: %.1f°C (threshold: %.1f°C)\n",
				alertData["value"], alertData["threshold"])
			fmt.Printf("   Satellite: %s, Sensor: %s\n",
				alertData["satellite_id"], alertData["sensor_id"])
			fmt.Printf("   Source: %s, Topic: %s\n", message.Source, message.Topic)

			// In a real system, this would trigger automated responses
			// such as sending commands to the satellite or notifying operators
			fmt.Println("   📋 Triggering automated response procedures...")

			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to alerts: %v", err)
	}

	fmt.Printf("🚨 Subscribed to temperature alerts with ID: %s\n", alertSubscription.ID)

	// Subscribe to system broadcasts
	broadcastSubscription, err := client.Subscribe(
		"/broadcast/system/+", // System-wide broadcasts
		types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
			var notification map[string]interface{}
			err := json.Unmarshal(message.Payload, &notification)
			if err != nil {
				return fmt.Errorf("failed to parse broadcast: %v", err)
			}

			fmt.Printf("📢 System Broadcast: %s\n", notification["title"])
			fmt.Printf("   Message: %s\n", notification["message"])
			fmt.Printf("   Type: %s, Severity: %s\n",
				notification["type"], notification["severity"])

			return nil
		}),
	)

	if err != nil {
		log.Fatalf("Failed to subscribe to broadcasts: %v", err)
	}

	fmt.Printf("📢 Subscribed to system broadcasts with ID: %s\n", broadcastSubscription.ID)

	// Show statistics periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats, err := client.GetClientStats()
			if err != nil {
				log.Printf("Failed to get client stats: %v", err)
				continue
			}

			fmt.Printf("\n📈 Client Statistics:\n")
			fmt.Printf("   Messages Received: %d\n", stats.MessagesReceived)
			fmt.Printf("   Bytes Received: %d\n", stats.BytesReceived)
			fmt.Printf("   Active Subscriptions: %d\n", stats.ActiveSubscriptions)
			fmt.Printf("   Connected Since: %d\n", stats.ConnectedAt)

			connInfo, err := client.GetConnectionInfo()
			if err == nil {
				fmt.Printf("   Connection Status: %s\n", connInfo.Status)
				fmt.Printf("   Protocol: %s\n", connInfo.Protocol)
			}
			fmt.Println()
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("🎯 Ground station monitoring is active. Press Ctrl+C to shutdown...")

	<-sigChan
	fmt.Println("\n🛑 Shutdown signal received. Cleaning up...")

	// Unsubscribe from all topics
	client.Unsubscribe(sensorSubscription.ID)
	client.Unsubscribe(alertSubscription.ID)
	client.Unsubscribe(broadcastSubscription.ID)

	fmt.Println("✅ Ground station subscriber shut down gracefully")
}