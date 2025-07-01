package test

import (
	"testing"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/serialization"
)

func TestBinarySerializer(t *testing.T) {
	serializer := serialization.NewBinarySerializer()
	
	// Create test message
	message := createTestMessage()
	
	// Test serialization
	data, err := serializer.Serialize(message)
	if err != nil {
		t.Fatalf("Serialization failed: %v", err)
	}
	
	if len(data) == 0 {
		t.Fatal("Serialized data is empty")
	}
	
	// Test deserialization
	deserializedMsg, err := serializer.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialization failed: %v", err)
	}
	
	// Verify basic fields
	if deserializedMsg.Topic != message.Topic {
		t.Errorf("Topic mismatch: expected %s, got %s", message.Topic, deserializedMsg.Topic)
	}
	
	if deserializedMsg.Type != message.Type {
		t.Errorf("Type mismatch: expected %d, got %d", message.Type, deserializedMsg.Type)
	}
	
	if string(deserializedMsg.Payload) != string(message.Payload) {
		t.Errorf("Payload mismatch: expected %s, got %s", string(message.Payload), string(deserializedMsg.Payload))
	}
}

func TestJSONSerializer(t *testing.T) {
	serializer := serialization.NewJSONSerializer()
	
	// Create test message
	message := createTestMessage()
	
	// Test serialization
	data, err := serializer.Serialize(message)
	if err != nil {
		t.Fatalf("Serialization failed: %v", err)
	}
	
	// Test deserialization
	deserializedMsg, err := serializer.Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialization failed: %v", err)
	}
	
	// Verify fields
	if deserializedMsg.Topic != message.Topic {
		t.Errorf("Topic mismatch: expected %s, got %s", message.Topic, deserializedMsg.Topic)
	}
}

func TestSerializationManager(t *testing.T) {
	manager := serialization.NewManager()
	
	// Create test message
	message := createTestMessage()
	
	// Test binary serialization
	data, err := manager.Serialize(message, serialization.FormatBinary)
	if err != nil {
		t.Fatalf("Binary serialization failed: %v", err)
	}
	
	deserializedMsg, err := manager.Deserialize(data, serialization.FormatBinary)
	if err != nil {
		t.Fatalf("Binary deserialization failed: %v", err)
	}
	
	if deserializedMsg.Topic != message.Topic {
		t.Errorf("Topic mismatch after binary serialization")
	}
	
	// Test JSON serialization
	data, err = manager.Serialize(message, serialization.FormatJSON)
	if err != nil {
		t.Fatalf("JSON serialization failed: %v", err)
	}
	
	deserializedMsg, err = manager.Deserialize(data, serialization.FormatJSON)
	if err != nil {
		t.Fatalf("JSON deserialization failed: %v", err)
	}
	
	if deserializedMsg.Topic != message.Topic {
		t.Errorf("Topic mismatch after JSON serialization")
	}
}

func TestAdaptiveSerialization(t *testing.T) {
	manager := serialization.NewManager()
	
	// Create test message
	message := createTestMessage()
	
	// Test adaptive serialization
	data, format, err := manager.SerializeAdaptive(message)
	if err != nil {
		t.Fatalf("Adaptive serialization failed: %v", err)
	}
	
	if len(data) == 0 {
		t.Fatal("Adaptive serialization returned empty data")
	}
	
	t.Logf("Adaptive serialization chose format: %d, size: %d bytes", format, len(data))
	
	// Test deserialization with detected format
	deserializedMsg, err := manager.Deserialize(data, format)
	if err != nil {
		t.Fatalf("Deserialization with adaptive format failed: %v", err)
	}
	
	if deserializedMsg.Topic != message.Topic {
		t.Errorf("Topic mismatch after adaptive serialization")
	}
}

func TestMessageFragmentation(t *testing.T) {
	fragmenter := serialization.NewMessageFragmenter()
	
	// Create large test message
	largeMessage := createLargeTestMessage()
	
	// Test fragmentation
	fragments, err := fragmenter.Fragment(largeMessage)
	if err != nil {
		t.Fatalf("Fragmentation failed: %v", err)
	}
	
	t.Logf("Message fragmented into %d fragments", len(fragments))
	
	// Verify fragment properties
	for i, fragment := range fragments {
		if fragment.FragmentIndex != i {
			t.Errorf("Fragment %d has wrong index: expected %d, got %d", i, i, fragment.FragmentIndex)
		}
		
		if fragment.TotalFragments != len(fragments) {
			t.Errorf("Fragment %d has wrong total count: expected %d, got %d", i, len(fragments), fragment.TotalFragments)
		}
		
		if i == len(fragments)-1 && !fragment.IsLast {
			t.Errorf("Last fragment not marked as last")
		}
		
		if i != len(fragments)-1 && fragment.IsLast {
			t.Errorf("Non-last fragment marked as last")
		}
	}
}

func TestMessageReassembly(t *testing.T) {
	fragmenter := serialization.NewMessageFragmenter()
	reassembler := serialization.NewMessageReassembler()
	
	// Create test message
	originalMessage := createLargeTestMessage()
	
	// Fragment the message
	fragments, err := fragmenter.Fragment(originalMessage)
	if err != nil {
		t.Fatalf("Fragmentation failed: %v", err)
	}
	
	// Reassemble fragments
	var reassembledMessage *types.SGCSFMessage
	for _, fragment := range fragments {
		message, err := reassembler.ProcessFragment(fragment)
		if err != nil {
			t.Fatalf("Fragment processing failed: %v", err)
		}
		
		if message != nil {
			if reassembledMessage != nil {
				t.Fatal("Multiple messages returned from reassembly")
			}
			reassembledMessage = message
		}
	}
	
	if reassembledMessage == nil {
		t.Fatal("No message returned from reassembly")
	}
	
	// Verify reassembled message
	if reassembledMessage.Topic != originalMessage.Topic {
		t.Errorf("Topic mismatch after reassembly")
	}
	
	if string(reassembledMessage.Payload) != string(originalMessage.Payload) {
		t.Errorf("Payload mismatch after reassembly")
	}
}

func TestCompression(t *testing.T) {
	lz4Compressor := serialization.NewLZ4Compressor()
	gzipCompressor := serialization.NewGzipCompressor()
	
	// Test data with repetition (good for compression)
	testData := []byte("This is a test string that repeats. This is a test string that repeats. This is a test string that repeats.")
	
	// Test LZ4 compression
	compressed, err := lz4Compressor.Compress(testData)
	if err != nil {
		t.Fatalf("LZ4 compression failed: %v", err)
	}
	
	decompressed, err := lz4Compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("LZ4 decompression failed: %v", err)
	}
	
	if string(decompressed) != string(testData) {
		t.Errorf("LZ4 decompression result mismatch")
	}
	
	t.Logf("LZ4: Original %d bytes, compressed %d bytes (%.1f%% ratio)",
		len(testData), len(compressed), float64(len(compressed))/float64(len(testData))*100)
	
	// Test Gzip compression
	compressed, err = gzipCompressor.Compress(testData)
	if err != nil {
		t.Fatalf("Gzip compression failed: %v", err)
	}
	
	decompressed, err = gzipCompressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Gzip decompression failed: %v", err)
	}
	
	if string(decompressed) != string(testData) {
		t.Errorf("Gzip decompression result mismatch")
	}
	
	t.Logf("Gzip: Original %d bytes, compressed %d bytes (%.1f%% ratio)",
		len(testData), len(compressed), float64(len(compressed))/float64(len(testData))*100)
}

func TestDataAnalyzer(t *testing.T) {
	analyzer := serialization.NewDataAnalyzer()
	
	// Test JSON data
	jsonData := []byte(`{"temperature": 25.6, "humidity": 65.2, "pressure": 1013.25}`)
	dataType := analyzer.AnalyzeDataType(jsonData)
	if dataType != "json" {
		t.Errorf("JSON data not detected correctly: got %s", dataType)
	}
	
	// Test text data
	textData := []byte("This is plain text data with some words and sentences.")
	dataType = analyzer.AnalyzeDataType(textData)
	if dataType != "text" {
		t.Errorf("Text data not detected correctly: got %s", dataType)
	}
	
	// Test binary data
	binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD, 0xFC}
	dataType = analyzer.AnalyzeDataType(binaryData)
	if dataType != "binary" {
		t.Errorf("Binary data not detected correctly: got %s", dataType)
	}
	
	// Test entropy calculation
	entropy := analyzer.CalculateEntropy(jsonData)
	if entropy < 0 || entropy > 1 {
		t.Errorf("Invalid entropy value: %f", entropy)
	}
	
	t.Logf("JSON data entropy: %.3f", entropy)
}

func BenchmarkBinarySerializer(b *testing.B) {
	serializer := serialization.NewBinarySerializer()
	message := createTestMessage()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := serializer.Serialize(message)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONSerializer(b *testing.B) {
	serializer := serialization.NewJSONSerializer()
	message := createTestMessage()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := serializer.Serialize(message)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerialization(b *testing.B) {
	message := createTestMessage()
	
	serializers := map[string]serialization.MessageSerializer{
		"Binary": serialization.NewBinarySerializer(),
		"JSON":   serialization.NewJSONSerializer(),
	}
	
	for name, serializer := range serializers {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				data, err := serializer.Serialize(message)
				if err != nil {
					b.Fatal(err)
				}
				_ = data
			}
		})
	}
}

// Helper functions

func createTestMessage() *types.SGCSFMessage {
	message := types.NewMessage()
	message.Topic = "/test/sensor/temperature"
	message.Type = types.MessageTypeAsync
	message.Source = "test-sensor"
	message.Priority = types.PriorityNormal
	message.QoS = types.QoSAtLeastOnce
	message.ContentType = "application/json"
	message.Payload = []byte(`{"temperature": 25.6, "humidity": 65.2}`)
	message.SetHeader("sensor-id", "temp-001")
	message.SetMetadata("location", "satellite")
	
	return message
}

func createLargeTestMessage() *types.SGCSFMessage {
	message := types.NewMessage()
	message.Topic = "/test/large/data"
	message.Type = types.MessageTypeAsync
	message.Source = "test-generator"
	
	// Create large payload (2KB)
	largePayload := make([]byte, 2048)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}
	message.Payload = largePayload
	
	return message
}

func createSyncTestMessage() *types.SGCSFMessage {
	message := types.NewMessage()
	message.Topic = "/test/sync/request"
	message.Type = types.MessageTypeSync
	message.IsSync = true
	message.RequestID = types.GenerateRequestID()
	message.Source = "test-client"
	message.Timeout = 30000 // 30 seconds
	message.Payload = []byte(`{"action": "get_data", "id": "12345"}`)
	
	return message
}

func TestSyncMessageSerialization(t *testing.T) {
	serializer := serialization.NewBinarySerializer()
	
	// Create sync message
	message := createSyncTestMessage()
	
	// Test serialization
	data, err := serializer.Serialize(message)
	if err != nil {
		t.Fatalf("Sync message serialization failed: %v", err)
	}
	
	// Test deserialization
	deserializedMsg, err := serializer.Deserialize(data)
	if err != nil {
		t.Fatalf("Sync message deserialization failed: %v", err)
	}
	
	// Verify sync-specific fields
	if !deserializedMsg.IsSync {
		t.Error("IsSync flag not preserved")
	}
	
	if deserializedMsg.Type != types.MessageTypeSync {
		t.Error("Message type not preserved")
	}
	
	if deserializedMsg.RequestID == "" {
		t.Error("Request ID not preserved")
	}
	
	if deserializedMsg.Timeout != message.Timeout {
		t.Errorf("Timeout not preserved: expected %d, got %d", message.Timeout, deserializedMsg.Timeout)
	}
}