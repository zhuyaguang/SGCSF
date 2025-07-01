package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
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
	fmt.Println("Starting SGCSF Sensor Data Publisher Example")

	// Create SGCSF client for satellite
	client := core.SatelliteClient("satellite-sat5-sensor", "localhost:7000")

	// Connect to SGCSF server
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to SGCSF server: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("Connected to SGCSF server")

	// Publish sensor data periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	sensorID := "temp-001"
	satelliteID := "sat5"

	for {
		select {
		case <-ticker.C:
			// Generate sensor data
			data := &SensorData{
				SensorID:    sensorID,
				Temperature: 20.0 + rand.Float64()*20.0, // 20-40°C
				Humidity:    40.0 + rand.Float64()*40.0, // 40-80%
				Pressure:    1000.0 + rand.Float64()*50.0, // 1000-1050 hPa
				Timestamp:   time.Now().Unix(),
				SatelliteID: satelliteID,
			}

			// Create SGCSF message
			message := core.NewMessage().
				Topic(fmt.Sprintf("/satellite/%s/sensors/temperature", satelliteID)).
				Type(types.MessageTypeAsync).
				Priority(types.PriorityNormal).
				QoS(types.QoSAtLeastOnce).
				ContentType("application/json").
				Source(fmt.Sprintf("satellite-%s-sensor", satelliteID)).
				Metadata("sensor_type", "temperature").
				Metadata("location", "satellite").
				TTL(10 * time.Minute).
				Payload(data).
				Build()

			// Publish the message
			err := client.Publish(message.Topic, message)
			if err != nil {
				log.Printf("Failed to publish sensor data: %v", err)
				continue
			}

			fmt.Printf("Published sensor data: %.1f°C, %.1f%%, %.1f hPa (from %s)\n",
				data.Temperature, data.Humidity, data.Pressure, data.SensorID)

			// Occasionally publish high priority alerts
			if data.Temperature > 35.0 {
				alertMessage := core.NewMessage().
					Topic(fmt.Sprintf("/satellite/%s/alerts/temperature", satelliteID)).
					Type(types.MessageTypeAsync).
					Priority(types.PriorityCritical).
					QoS(types.QoSExactlyOnce).
					ContentType("application/json").
					Source(fmt.Sprintf("satellite-%s-sensor", satelliteID)).
					Metadata("alert_type", "high_temperature").
					Metadata("severity", "critical").
					TTL(30 * time.Minute).
					Payload(map[string]interface{}{
						"alert":      "HIGH_TEMPERATURE",
						"value":      data.Temperature,
						"threshold":  35.0,
						"sensor_id":  data.SensorID,
						"timestamp":  data.Timestamp,
						"satellite_id": data.SatelliteID,
					}).
					Build()

				err := client.Publish(alertMessage.Topic, alertMessage)
				if err != nil {
					log.Printf("Failed to publish temperature alert: %v", err)
				} else {
					fmt.Printf("🚨 ALERT: High temperature detected: %.1f°C\n", data.Temperature)
				}
			}

		case <-ctx.Done():
			fmt.Println("Shutting down sensor publisher...")
			return
		}
	}
}