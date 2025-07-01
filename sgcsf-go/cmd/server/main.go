package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sgcsf/sgcsf-go/pkg/transport"
)

func main() {
	fmt.Println("🚀 Starting SGCSF Server")

	// Create QUIC transport
	quicConfig := &transport.QUICConfig{
		ListenAddr:       ":7000",
		MaxStreams:       1000,
		KeepAlive:        30 * time.Second,
		HandshakeTimeout: 10 * time.Second,
		IdleTimeout:      5 * time.Minute,
		MaxPacketSize:    1350,
		EnableRetries:    true,
		MaxRetries:       3,
	}

	quicTransport := transport.NewQUICTransport(quicConfig)

	// Start listening for connections
	ctx := context.Background()
	err := quicTransport.Listen(ctx)
	if err != nil {
		log.Fatalf("Failed to start QUIC transport: %v", err)
	}

	fmt.Println("✅ SGCSF Server started successfully")
	fmt.Printf("📡 Listening on %s\n", quicConfig.ListenAddr)
	fmt.Println("🎯 Ready to accept satellite and ground station connections")

	// Show server statistics periodically
	go showServerStats(quicTransport)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to shutdown the server...")

	<-sigChan
	fmt.Println("\n🛑 Shutdown signal received. Stopping server...")

	// Stop transport
	err = quicTransport.Stop()
	if err != nil {
		log.Printf("Error stopping transport: %v", err)
	}

	fmt.Println("✅ SGCSF Server shut down gracefully")
}

func showServerStats(transport *transport.QUICTransport) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		connections := transport.GetConnections()
		connectionCount := transport.GetConnectionCount()

		fmt.Printf("\n📈 Server Statistics (%s):\n", time.Now().Format("15:04:05"))
		fmt.Printf("   Active Connections: %d\n", connectionCount)

		if connectionCount > 0 {
			fmt.Println("   Connected Clients:")
			for id, conn := range connections {
				stats := conn.GetStats()
				fmt.Printf("     - %s (from %s)\n", id, conn.GetRemoteAddr())
				fmt.Printf("       Packets: %d sent, %d received\n", 
					stats.PacketsSent, stats.PacketsReceived)
				fmt.Printf("       Bytes: %d sent, %d received\n", 
					stats.BytesSent, stats.BytesReceived)
				fmt.Printf("       Connected: %v\n", 
					time.Since(stats.ConnectedAt).Round(time.Second))
			}
		} else {
			fmt.Println("   No active connections")
		}
		fmt.Println()
	}
}