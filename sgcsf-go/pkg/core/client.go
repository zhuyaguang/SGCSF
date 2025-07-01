package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/serialization"
)

// Client implements SGCSFClient interface
type Client struct {
	config          *types.ClientConfig
	connection      ClientConnection
	topicManager    TopicManager
	messageRouter   MessageRouter
	streamManager   StreamManager
	qosManager      QoSManager
	serialManager   serialization.SerializationManager
	fragmenter      *serialization.MessageFragmenter
	reassembler     *serialization.MessageReassembler
	
	subscriptions   map[string]*types.Subscription
	syncHandlers    map[string]chan *types.SGCSFMessage
	stats           *types.ClientStats
	
	connected       bool
	mutex           sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewClient creates a new SGCSF client
func NewClient(config *types.ClientConfig) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	
	client := &Client{
		config:        config,
		subscriptions: make(map[string]*types.Subscription),
		syncHandlers:  make(map[string]chan *types.SGCSFMessage),
		stats: &types.ClientStats{
			ConnectedAt: time.Now().Unix(),
		},
		serialManager: serialization.NewManager(),
		fragmenter:    serialization.NewMessageFragmenter(),
		reassembler:   serialization.NewMessageReassembler(),
		ctx:           ctx,
		cancel:        cancel,
	}
	
	// Initialize managers
	client.topicManager = NewTopicManager()
	client.messageRouter = NewMessageRouter(client)
	client.streamManager = NewStreamManager()
	client.qosManager = NewQoSManager()
	
	return client
}

// Connect establishes connection to SGCSF server
func (c *Client) Connect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if c.connected {
		return nil
	}
	
	// Create connection (this would use QUIC transport in real implementation)
	conn, err := c.createConnection()
	if err != nil {
		return fmt.Errorf("failed to create connection: %v", err)
	}
	
	c.connection = conn
	c.connected = true
	c.stats.ConnectedAt = time.Now().Unix()
	
	// Start message processing goroutines
	go c.messageProcessingLoop()
	go c.heartbeatLoop()
	
	return nil
}

// Disconnect closes the connection
func (c *Client) Disconnect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	if !c.connected {
		return nil
	}
	
	c.cancel() // Cancel context to stop goroutines
	
	if c.connection != nil {
		c.connection.Close()
	}
	
	c.connected = false
	return nil
}

// IsConnected returns connection status
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected
}

// Publish sends an async message
func (c *Client) Publish(topic string, message *types.SGCSFMessage) error {
	if !c.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	
	// Set message properties
	message.Topic = topic
	message.Source = c.config.ClientID
	message.Type = types.MessageTypeAsync
	
	// Handle QoS
	err := c.qosManager.HandleMessage(message)
	if err != nil {
		return err
	}
	
	// Fragment and send
	return c.sendMessage(message)
}

// Subscribe subscribes to a topic
func (c *Client) Subscribe(topic string, handler types.MessageHandler) (*types.Subscription, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}
	
	subscription := &types.Subscription{
		ID:        types.GenerateSubscriptionID(),
		Topic:     topic,
		ClientID:  c.config.ClientID,
		QoS:       c.config.DefaultQoS,
		CreatedAt: time.Now().Unix(),
		Handler:   handler,
		Metadata:  make(map[string]string),
	}
	
	c.mutex.Lock()
	c.subscriptions[subscription.ID] = subscription
	c.mutex.Unlock()
	
	// Register with topic manager
	_, err := c.topicManager.Subscribe(c.config.ClientID, topic, handler)
	if err != nil {
		c.mutex.Lock()
		delete(c.subscriptions, subscription.ID)
		c.mutex.Unlock()
		return nil, err
	}
	
	c.stats.ActiveSubscriptions++
	return subscription, nil
}

// Unsubscribe removes a subscription
func (c *Client) Unsubscribe(subscriptionID string) error {
	c.mutex.Lock()
	_, exists := c.subscriptions[subscriptionID]
	if exists {
		delete(c.subscriptions, subscriptionID)
	}
	c.mutex.Unlock()
	
	if !exists {
		return fmt.Errorf("subscription not found: %s", subscriptionID)
	}
	
	err := c.topicManager.Unsubscribe(subscriptionID)
	if err == nil {
		c.stats.ActiveSubscriptions--
	}
	return err
}

// PublishSync sends a synchronous message and waits for response
func (c *Client) PublishSync(topic string, message *types.SGCSFMessage, timeout time.Duration) (*types.SGCSFMessage, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}
	
	// Set message properties
	message.Topic = topic
	message.Source = c.config.ClientID
	message.Type = types.MessageTypeSync
	message.IsSync = true
	message.RequestID = types.GenerateRequestID()
	message.Timeout = timeout.Milliseconds()
	
	// Create response channel
	responseChan := make(chan *types.SGCSFMessage, 1)
	c.mutex.Lock()
	c.syncHandlers[message.RequestID] = responseChan
	c.mutex.Unlock()
	
	// Send message
	err := c.sendMessage(message)
	if err != nil {
		c.mutex.Lock()
		delete(c.syncHandlers, message.RequestID)
		c.mutex.Unlock()
		return nil, err
	}
	
	// Wait for response or timeout
	select {
	case response := <-responseChan:
		c.mutex.Lock()
		delete(c.syncHandlers, message.RequestID)
		c.mutex.Unlock()
		return response, nil
	case <-time.After(timeout):
		c.mutex.Lock()
		delete(c.syncHandlers, message.RequestID)
		c.mutex.Unlock()
		return nil, fmt.Errorf("sync request timeout")
	case <-c.ctx.Done():
		c.mutex.Lock()
		delete(c.syncHandlers, message.RequestID)
		c.mutex.Unlock()
		return nil, fmt.Errorf("client disconnected")
	}
}

// SendResponse sends a response to a sync message
func (c *Client) SendResponse(originalMessage *types.SGCSFMessage, response *types.SGCSFMessage) error {
	if !c.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	
	// Set response properties
	response.Type = types.MessageTypeResponse
	response.Source = c.config.ClientID
	response.Destination = originalMessage.Source
	response.ResponseTo = originalMessage.RequestID
	response.Topic = originalMessage.Topic
	
	return c.sendMessage(response)
}

// CreateStream creates a new data stream
func (c *Client) CreateStream(topic string, streamType types.StreamType) (Stream, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}
	
	streamID := types.GenerateStreamID()
	stream, err := c.streamManager.CreateStream(streamID, streamType, c.connection)
	if err != nil {
		return nil, err
	}
	
	// Send stream start message
	message := &types.SGCSFMessage{
		ID:            types.GenerateMessageID(),
		Topic:         topic,
		Type:          types.MessageTypeStream,
		Source:        c.config.ClientID,
		Timestamp:     time.Now().UnixNano(),
		StreamID:      streamID,
		StreamType:    streamType,
		IsStreamStart: true,
	}
	
	err = c.sendMessage(message)
	if err != nil {
		stream.Close()
		return nil, err
	}
	
	c.stats.ActiveStreams++
	return stream, nil
}

// AcceptStream accepts an incoming stream
func (c *Client) AcceptStream(subscription *types.Subscription) (Stream, error) {
	// This would be implemented to accept incoming streams
	// For now, return a placeholder
	return nil, fmt.Errorf("AcceptStream not implemented yet")
}

// Broadcast sends a message to all subscribers
func (c *Client) Broadcast(topic string, message *types.SGCSFMessage) error {
	if !c.IsConnected() {
		return fmt.Errorf("client not connected")
	}
	
	// Set message properties
	message.Topic = topic
	message.Source = c.config.ClientID
	message.Type = types.MessageTypeBroadcast
	
	return c.sendMessage(message)
}

// CreateTopic creates a new topic
func (c *Client) CreateTopic(topic string, config *types.TopicConfig) error {
	return c.topicManager.CreateTopic(topic, config)
}

// GetTopicInfo gets information about a topic
func (c *Client) GetTopicInfo(topic string) (*types.TopicInfo, error) {
	topicObj, err := c.topicManager.GetTopic(topic)
	if err != nil {
		return nil, err
	}
	
	return &types.TopicInfo{
		Config:       topicObj.Config,
		Subscribers:  len(topicObj.Subscribers),
		MessageCount: topicObj.MessageCount,
		TotalSize:    topicObj.TotalSize,
		CreatedAt:    topicObj.CreatedAt,
		UpdatedAt:    topicObj.UpdatedAt,
	}, nil
}

// ListSubscriptions lists all subscriptions
func (c *Client) ListSubscriptions() ([]*types.Subscription, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	subscriptions := make([]*types.Subscription, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subscriptions = append(subscriptions, sub)
	}
	
	return subscriptions, nil
}

// GetClientStats returns client statistics
func (c *Client) GetClientStats() (*types.ClientStats, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	
	// Create a copy of stats
	stats := *c.stats
	stats.LastActivity = time.Now().Unix()
	return &stats, nil
}

// GetConnectionInfo returns connection information
func (c *Client) GetConnectionInfo() (*types.ConnectionInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("client not connected")
	}
	
	return &types.ConnectionInfo{
		Status:      "connected",
		RemoteAddr:  c.config.ServerAddr,
		ConnectedAt: c.stats.ConnectedAt,
		Protocol:    "QUIC",
	}, nil
}

// sendMessage fragments and sends a message
func (c *Client) sendMessage(message *types.SGCSFMessage) error {
	// Fragment the message
	fragments, err := c.fragmenter.Fragment(message)
	if err != nil {
		return fmt.Errorf("fragmentation failed: %v", err)
	}
	
	// Send each fragment
	for _, fragment := range fragments {
		err := c.connection.SendFragment(fragment)
		if err != nil {
			return fmt.Errorf("failed to send fragment: %v", err)
		}
	}
	
	c.stats.MessagesSent++
	c.stats.BytesSent += int64(len(message.Payload))
	
	return nil
}

// createConnection creates a connection to the server
func (c *Client) createConnection() (ClientConnection, error) {
	// This would create a real QUIC connection in production
	// For now, return a mock connection
	return NewMockClientConnection(c.config.ClientID, c.config.ServerAddr), nil
}

// messageProcessingLoop processes incoming messages
func (c *Client) messageProcessingLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Process incoming messages and fragments
			time.Sleep(100 * time.Millisecond) // Placeholder
		}
	}
}

// heartbeatLoop sends periodic heartbeats
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(time.Duration(c.config.KeepAlive) * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.IsConnected() {
				c.connection.Ping()
			}
		}
	}
}

// handleSyncResponse handles incoming sync responses
func (c *Client) handleSyncResponse(response *types.SGCSFMessage) {
	c.mutex.RLock()
	responseChan, exists := c.syncHandlers[response.ResponseTo]
	c.mutex.RUnlock()
	
	if exists {
		select {
		case responseChan <- response:
		default:
			// Channel full or closed
		}
	}
}