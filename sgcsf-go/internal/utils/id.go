package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var messageCounter uint64

// GenerateMessageID generates a unique message ID
func GenerateMessageID() string {
	// Use atomic counter + timestamp + random bytes for uniqueness
	counter := atomic.AddUint64(&messageCounter, 1)
	timestamp := time.Now().UnixNano()
	
	// Generate 4 random bytes
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("msg_%d_%d_%s", 
		timestamp, 
		counter, 
		hex.EncodeToString(randomBytes))
}

// GenerateRequestID generates a unique request ID for sync messages
func GenerateRequestID() string {
	counter := atomic.AddUint64(&messageCounter, 1)
	timestamp := time.Now().UnixNano()
	
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("req_%d_%d_%s", 
		timestamp, 
		counter, 
		hex.EncodeToString(randomBytes))
}

// GenerateStreamID generates a unique stream ID
func GenerateStreamID() string {
	counter := atomic.AddUint64(&messageCounter, 1)
	timestamp := time.Now().UnixNano()
	
	randomBytes := make([]byte, 6)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("stream_%d_%d_%s", 
		timestamp, 
		counter, 
		hex.EncodeToString(randomBytes))
}

// GenerateClientID generates a unique client ID
func GenerateClientID(prefix string) string {
	timestamp := time.Now().UnixNano()
	
	randomBytes := make([]byte, 6)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("%s_%d_%s", 
		prefix, 
		timestamp, 
		hex.EncodeToString(randomBytes))
}

// GenerateSubscriptionID generates a unique subscription ID
func GenerateSubscriptionID() string {
	counter := atomic.AddUint64(&messageCounter, 1)
	timestamp := time.Now().UnixNano()
	
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	
	return fmt.Sprintf("sub_%d_%d_%s", 
		timestamp, 
		counter, 
		hex.EncodeToString(randomBytes))
}