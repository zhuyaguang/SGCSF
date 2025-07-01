package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/core"
)

func TestClientConnection(t *testing.T) {
	// Create test client
	config := &types.ClientConfig{
		ClientID:   "test-client",
		ServerAddr: "localhost:7000",
	}
	client := core.NewClient(config)
	
	// Test connection
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Client connection failed: %v", err)
	}
	
	if !client.IsConnected() {
		t.Error("Client should be connected")
	}
	
	// Test disconnection
	err = client.Disconnect()
	if err != nil {
		t.Fatalf("Client disconnection failed: %v", err)
	}
	
	if client.IsConnected() {
		t.Error("Client should be disconnected")
	}
}

func TestMessagePublish(t *testing.T) {
	config := &types.ClientConfig{
		ClientID:   "test-publisher",
		ServerAddr: "localhost:7000",
	}
	client := core.NewClient(config)
	
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Client connection failed: %v", err)
	}
	defer client.Disconnect()
	
	// Create test message
	message := types.NewMessage()
	message.Topic = "/test/publish"
	message.Payload = []byte("test payload")
	
	// Test async publish
	err = client.Publish("/test/publish", message)
	if err != nil {
		t.Fatalf("Message publish failed: %v", err)
	}
}

func TestSubscription(t *testing.T) {
	config := &types.ClientConfig{
		ClientID:   "test-subscriber",
		ServerAddr: "localhost:7000",
	}
	client := core.NewClient(config)
	
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Client connection failed: %v", err)
	}
	defer client.Disconnect()
	
	// Create message handler
	receivedMessages := make(chan *types.SGCSFMessage, 10)
	handler := types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		receivedMessages <- message
		return nil
	})
	
	// Test subscription
	subscription, err := client.Subscribe("/test/subscription", handler)
	if err != nil {
		t.Fatalf("Subscription failed: %v", err)
	}
	
	if subscription.Topic != "/test/subscription" {
		t.Errorf("Subscription topic mismatch: expected /test/subscription, got %s", subscription.Topic)
	}
	
	// Test unsubscription
	err = client.Unsubscribe(subscription.ID)
	if err != nil {
		t.Fatalf("Unsubscription failed: %v", err)
	}
}

func TestTopicManager(t *testing.T) {
	topicManager := core.NewTopicManager()
	
	// Test topic creation
	topicConfig := &types.TopicConfig{
		Name:       "/test/topic",
		Type:       types.TopicUnicast,
		Persistent: true,
		Retention:  3600,
		MaxSize:    1024 * 1024,
		QoS:        types.QoSAtLeastOnce,
	}
	
	err := topicManager.CreateTopic("/test/topic", topicConfig)
	if err != nil {
		t.Fatalf("Topic creation failed: %v", err)
	}
	
	// Test topic existence
	if !topicManager.TopicExists("/test/topic") {
		t.Error("Topic should exist")
	}
	
	// Test topic retrieval
	topic, err := topicManager.GetTopic("/test/topic")
	if err != nil {
		t.Fatalf("Topic retrieval failed: %v", err)
	}
	
	if topic.Config.Name != "/test/topic" {
		t.Errorf("Topic name mismatch: expected /test/topic, got %s", topic.Config.Name)
	}
	
	// Test subscription
	handler := types.MessageHandlerFunc(func(message *types.SGCSFMessage) error {
		return nil
	})
	
	_, err = topicManager.Subscribe("test-client", "/test/topic", handler)
	if err != nil {
		t.Fatalf("Subscription failed: %v", err)
	}
	
	// Test topic deletion
	err = topicManager.DeleteTopic("/test/topic")
	if err != nil {
		t.Fatalf("Topic deletion failed: %v", err)
	}
	
	if topicManager.TopicExists("/test/topic") {
		t.Error("Topic should not exist after deletion")
	}
}

func TestTopicMatching(t *testing.T) {
	topicManager := core.NewTopicManager()
	
	testCases := []struct {
		pattern  string
		topic    string
		expected bool
	}{
		{"/test/exact", "/test/exact", true},
		{"/test/+/sensor", "/test/sat5/sensor", true},
		{"/test/+/sensor", "/test/sat5/actuator", false},
		{"/test/satellite/#", "/test/satellite/sat5/sensor", true},
		{"/test/satellite/#", "/test/ground/station", false},
		{"/test/*/data", "/test/anything/data", true},
	}
	
	for _, tc := range testCases {
		result := topicManager.MatchTopic(tc.pattern, tc.topic)
		if result != tc.expected {
			t.Errorf("Topic matching failed for pattern %s and topic %s: expected %v, got %v",
				tc.pattern, tc.topic, tc.expected, result)
		}
	}
}

func TestMessageBuilder(t *testing.T) {
	// Test basic message building
	message := core.NewMessage().
		Topic("/test/builder").
		Type(types.MessageTypeAsync).
		Priority(types.PriorityHigh).
		QoS(types.QoSExactlyOnce).
		TTL(5 * time.Minute).
		Header("test-header", "test-value").
		Metadata("test-meta", "test-value").
		ContentType("application/json").
		Payload(map[string]string{"key": "value"}).
		Build()
	
	if message.Topic != "/test/builder" {
		t.Errorf("Topic not set correctly: expected /test/builder, got %s", message.Topic)
	}
	
	if message.Type != types.MessageTypeAsync {
		t.Errorf("Type not set correctly: expected %d, got %d", types.MessageTypeAsync, message.Type)
	}
	
	if message.Priority != types.PriorityHigh {
		t.Errorf("Priority not set correctly: expected %d, got %d", types.PriorityHigh, message.Priority)
	}
	
	if message.QoS != types.QoSExactlyOnce {
		t.Errorf("QoS not set correctly: expected %d, got %d", types.QoSExactlyOnce, message.QoS)
	}
	
	if message.ContentType != "application/json" {
		t.Errorf("ContentType not set correctly: expected application/json, got %s", message.ContentType)
	}
	
	headerValue, exists := message.GetHeader("test-header")
	if !exists || headerValue != "test-value" {
		t.Errorf("Header not set correctly: expected test-value, got %s", headerValue)
	}
	
	metaValue, exists := message.GetMetadata("test-meta")
	if !exists || metaValue != "test-value" {
		t.Errorf("Metadata not set correctly: expected test-value, got %s", metaValue)
	}
}

func TestSyncMessage(t *testing.T) {
	// Test sync message building
	message := core.NewMessage().
		Topic("/test/sync").
		Type(types.MessageTypeSync).
		Timeout(30 * time.Second).
		Payload("sync request").
		Build()
	
	if !message.IsSync {
		t.Error("IsSync should be true for sync messages")
	}
	
	if message.RequestID == "" {
		t.Error("RequestID should be set for sync messages")
	}
	
	if message.Timeout != 30000 {
		t.Errorf("Timeout not set correctly: expected 30000, got %d", message.Timeout)
	}
}

func TestQuickMessages(t *testing.T) {
	// Test quick message creation
	asyncMsg := core.QuickMessage("/test/quick", "test payload")
	if asyncMsg.Type != types.MessageTypeAsync {
		t.Error("Quick message should be async by default")
	}
	
	syncMsg := core.QuickSyncMessage("/test/sync", "sync payload", 10*time.Second)
	if syncMsg.Type != types.MessageTypeSync {
		t.Error("Quick sync message should be sync type")
	}
	
	broadcastMsg := core.QuickBroadcast("/test/broadcast", "broadcast payload")
	if broadcastMsg.Type != types.MessageTypeBroadcast {
		t.Error("Quick broadcast message should be broadcast type")
	}
	
	// Test response message
	responseMsg := core.QuickResponse(syncMsg, "response payload")
	if responseMsg.Type != types.MessageTypeResponse {
		t.Error("Quick response should be response type")
	}
	
	if responseMsg.ResponseTo != syncMsg.RequestID {
		t.Error("Response message should reference original request ID")
	}
}

func TestClientBuilder(t *testing.T) {
	// Test client builder
	client := core.NewClientBuilder().
		ClientID("test-client").
		ServerAddr("localhost:7000").
		ConnTimeout(20 * time.Second).
		KeepAlive(45 * time.Second).
		MaxStreams(500).
		DefaultQoS(types.QoSExactlyOnce).
		Debug(true).
		Metadata("environment", "test").
		Build()
	
	config, err := client.GetClientStats()
	if err != nil {
		t.Fatalf("Failed to get client stats: %v", err)
	}
	
	if config == nil {
		t.Error("Client stats should not be nil")
	}
}

func TestQuickClients(t *testing.T) {
	// Test quick client creation
	quickClient := core.QuickClient("quick-client", "localhost:7000")
	if !quickClient.IsConnected() {
		// This is expected since we haven't actually connected
	}
	
	satelliteClient := core.SatelliteClient("sat-client", "localhost:7000")
	if satelliteClient == nil {
		t.Error("Satellite client should not be nil")
	}
	
	groundClient := core.GroundClient("ground-client", "localhost:7000")
	if groundClient == nil {
		t.Error("Ground client should not be nil")
	}
}

func TestConcurrentOperations(t *testing.T) {
	topicManager := core.NewTopicManager()
	
	// Test concurrent topic operations
	var wg sync.WaitGroup
	numRoutines := 10
	
	// Concurrent topic creation
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			topicConfig := &types.TopicConfig{
				Name:       fmt.Sprintf("/test/concurrent/%d", id),
				Type:       types.TopicUnicast,
				Persistent: false,
				Retention:  3600,
				MaxSize:    1024,
				QoS:        types.QoSAtLeastOnce,
			}
			
			err := topicManager.CreateTopic(topicConfig.Name, topicConfig)
			if err != nil {
				t.Errorf("Concurrent topic creation failed: %v", err)
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify all topics were created
	topics := topicManager.ListTopics()
	if len(topics) < numRoutines {
		t.Errorf("Expected at least %d topics, got %d", numRoutines, len(topics))
	}
}

func TestMessageValidation(t *testing.T) {
	builder := core.NewMessage()
	
	// Test validation without required fields
	err := builder.Validate()
	if err == nil {
		t.Error("Validation should fail for message without topic")
	}
	
	// Add topic and test again
	builder.Topic("/test/validation")
	err = builder.Validate()
	if err == nil {
		t.Error("Validation should fail for message without source")
	}
	
	// Add source and test again
	builder.Source("test-source")
	err = builder.Validate()
	if err != nil {
		t.Errorf("Validation should pass: %v", err)
	}
	
	// Test sync message validation
	builder.Type(types.MessageTypeSync)
	err = builder.Validate()
	if err == nil {
		t.Error("Validation should fail for sync message without request ID")
	}
	
	builder.RequestID("test-request-id")
	err = builder.Validate()
	if err != nil {
		t.Errorf("Validation should pass for valid sync message: %v", err)
	}
}

func TestMessageCloning(t *testing.T) {
	originalMessage := core.NewMessage().
		Topic("/test/clone").
		Type(types.MessageTypeAsync).
		Priority(types.PriorityHigh).
		Header("test", "value").
		Metadata("meta", "data").
		Payload("test payload").
		Build()
	
	clonedMessage := originalMessage.Clone()
	
	// Verify clone has same values
	if clonedMessage.Topic != originalMessage.Topic {
		t.Error("Cloned message topic mismatch")
	}
	
	if clonedMessage.Type != originalMessage.Type {
		t.Error("Cloned message type mismatch")
	}
	
	if string(clonedMessage.Payload) != string(originalMessage.Payload) {
		t.Error("Cloned message payload mismatch")
	}
	
	// Verify clone is independent
	clonedMessage.Topic = "/test/modified"
	if originalMessage.Topic == "/test/modified" {
		t.Error("Original message should not be affected by clone modification")
	}
	
	clonedMessage.SetHeader("test", "modified")
	if originalMessage.Headers["test"] == "modified" {
		t.Error("Original message headers should not be affected by clone modification")
	}
}