package core

import (
	"context"
	"io"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// SGCSFClient defines the main client interface for SGCSF
type SGCSFClient interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	// Async publish/subscribe
	Publish(topic string, message *types.SGCSFMessage) error
	Subscribe(topic string, handler types.MessageHandler) (*types.Subscription, error)
	Unsubscribe(subscriptionID string) error

	// Sync communication
	PublishSync(topic string, message *types.SGCSFMessage, timeout time.Duration) (*types.SGCSFMessage, error)
	SendResponse(originalMessage *types.SGCSFMessage, response *types.SGCSFMessage) error

	// Stream communication
	CreateStream(topic string, streamType types.StreamType) (Stream, error)
	AcceptStream(subscription *types.Subscription) (Stream, error)

	// Broadcast communication
	Broadcast(topic string, message *types.SGCSFMessage) error

	// Topic management
	CreateTopic(topic string, config *types.TopicConfig) error
	GetTopicInfo(topic string) (*types.TopicInfo, error)
	ListSubscriptions() ([]*types.Subscription, error)

	// Monitoring and diagnostics
	GetClientStats() (*types.ClientStats, error)
	GetConnectionInfo() (*types.ConnectionInfo, error)
}

// Stream defines the interface for streaming data
type Stream interface {
	io.ReadWriteCloser

	// Stream control
	SetWriteDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error

	// Stream information
	GetStreamID() string
	GetStreamType() types.StreamType
	GetStreamStats() (*types.StreamStats, error)

	// Stream operations
	Flush() error
	CloseWrite() error
}

// SGCSFServer defines the server interface
type SGCSFServer interface {
	// Server lifecycle
	Start(ctx context.Context) error
	Stop() error
	IsRunning() bool

	// Client management
	RegisterClient(clientID string, client ClientConnection) error
	UnregisterClient(clientID string) error
	GetClient(clientID string) (ClientConnection, error)
	ListClients() []string

	// Topic management
	CreateTopic(topic string, config *types.TopicConfig) error
	DeleteTopic(topic string) error
	GetTopic(topic string) (*types.TopicInfo, error)
	ListTopics() []*types.TopicInfo

	// Message routing
	RouteMessage(message *types.SGCSFMessage) error
	BroadcastMessage(topic string, message *types.SGCSFMessage) error

	// Server statistics
	GetServerStats() (*ServerStats, error)
}

// ClientConnection represents a connection to a client
type ClientConnection interface {
	// Connection info
	GetClientID() string
	GetRemoteAddr() string
	IsConnected() bool
	GetConnectedAt() time.Time

	// Message handling
	SendMessage(message *types.SGCSFMessage) error
	SendFragment(fragment *types.Fragment) error

	// Stream handling
	CreateStream(streamID string, streamType types.StreamType) (Stream, error)
	AcceptStream(streamID string) (Stream, error)

	// Connection management
	Close() error
	Ping() error

	// Statistics
	GetStats() (*types.ClientStats, error)
}

// TopicManager manages topics and subscriptions
type TopicManager interface {
	// Topic lifecycle
	CreateTopic(name string, config *types.TopicConfig) error
	DeleteTopic(name string) error
	GetTopic(name string) (*Topic, error)
	ListTopics() []*Topic
	TopicExists(name string) bool

	// Subscription management
	Subscribe(clientID string, topic string, handler types.MessageHandler) (*types.Subscription, error)
	Unsubscribe(subscriptionID string) error
	GetSubscription(subscriptionID string) (*types.Subscription, error)
	ListSubscriptions(clientID string) []*types.Subscription
	GetTopicSubscribers(topic string) []*types.Subscription

	// Message routing
	RouteMessage(message *types.SGCSFMessage) ([]*types.Subscription, error)
	MatchTopic(pattern string, topic string) bool
}

// MessageRouter routes messages to appropriate handlers
type MessageRouter interface {
	// Route messages
	RouteMessage(message *types.SGCSFMessage) error
	RouteFragment(fragment *types.Fragment) error

	// Sync message handling
	RegisterSyncHandler(requestID string, handler SyncHandler) error
	HandleSyncResponse(response *types.SGCSFMessage) error

	// Stream routing
	RouteStreamMessage(message *types.SGCSFMessage) error
}

// SyncHandler handles synchronous message responses
type SyncHandler interface {
	HandleResponse(response *types.SGCSFMessage) error
	HandleTimeout() error
	HandleError(err error) error
}

// StreamManager manages data streams
type StreamManager interface {
	// Stream lifecycle
	CreateStream(streamID string, streamType types.StreamType, clientConn ClientConnection) (Stream, error)
	GetStream(streamID string) (Stream, error)
	CloseStream(streamID string) error
	ListStreams() []string

	// Stream operations
	RegisterStreamHandler(streamType types.StreamType, handler StreamHandler) error
	RouteStreamData(streamID string, data []byte) error
}

// StreamHandler handles stream events
type StreamHandler interface {
	OnStreamCreated(stream Stream) error
	OnStreamData(stream Stream, data []byte) error
	OnStreamClosed(stream Stream) error
	OnStreamError(stream Stream, err error) error
}

// QoSManager manages Quality of Service
type QoSManager interface {
	// Message QoS
	HandleMessage(message *types.SGCSFMessage) error
	EnsureDelivery(message *types.SGCSFMessage, qos types.QoSLevel) error
	AcknowledgeMessage(messageID string) error

	// Priority handling
	PrioritizeMessage(message *types.SGCSFMessage) int
	ScheduleMessage(message *types.SGCSFMessage) error
}

// Topic represents a communication topic
type Topic struct {
	Config        *types.TopicConfig
	Subscribers   []*types.Subscription
	MessageCount  int64
	TotalSize     int64
	CreatedAt     int64
	UpdatedAt     int64
}

// ServerStats represents server statistics
type ServerStats struct {
	StartTime       int64 `json:"start_time"`
	Uptime          int64 `json:"uptime"`
	ClientCount     int   `json:"client_count"`
	TopicCount      int   `json:"topic_count"`
	MessageCount    int64 `json:"message_count"`
	BytesProcessed  int64 `json:"bytes_processed"`
	ActiveStreams   int   `json:"active_streams"`
	ErrorCount      int64 `json:"error_count"`
	MemoryUsage     int64 `json:"memory_usage"`
	CPUUsage        float64 `json:"cpu_usage"`
}