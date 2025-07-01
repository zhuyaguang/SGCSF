package types

import (
	"github.com/sgcsf/sgcsf-go/internal/utils"
)

// Re-export utils functions for convenience
var (
	GenerateMessageID      = utils.GenerateMessageID
	GenerateRequestID      = utils.GenerateRequestID  
	GenerateStreamID       = utils.GenerateStreamID
	GenerateClientID       = utils.GenerateClientID
	GenerateSubscriptionID = utils.GenerateSubscriptionID
)

// Topic represents a communication topic
type Topic struct {
	Name        string            `json:"name"`
	Type        TopicType         `json:"type"`
	Persistent  bool              `json:"persistent"`
	Retention   int64             `json:"retention"`    // Retention time in seconds
	MaxSize     int64             `json:"max_size"`     // Max size in bytes
	QoS         QoSLevel          `json:"qos"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

// TopicType defines the type of topic
type TopicType uint8

const (
	TopicUnicast   TopicType = iota // 单播主题
	TopicBroadcast                  // 广播主题
	TopicMulticast                  // 组播主题
)

// Subscription represents a topic subscription
type Subscription struct {
	ID          string            `json:"id"`
	Topic       string            `json:"topic"`
	ClientID    string            `json:"client_id"`
	QoS         QoSLevel          `json:"qos"`
	CreatedAt   int64             `json:"created_at"`
	LastMessage int64             `json:"last_message"`
	Metadata    map[string]string `json:"metadata"`
	Handler     MessageHandler    `json:"-"` // Not serialized
}

// MessageHandler defines the interface for handling messages
type MessageHandler interface {
	Handle(message *SGCSFMessage) error
}

// MessageHandlerFunc is a function type that implements MessageHandler
type MessageHandlerFunc func(message *SGCSFMessage) error

// Handle implements MessageHandler interface
func (f MessageHandlerFunc) Handle(message *SGCSFMessage) error {
	return f(message)
}

// ClientConfig represents client configuration
type ClientConfig struct {
	ClientID      string            `json:"client_id"`
	ServerAddr    string            `json:"server_addr"`
	ConnTimeout   int               `json:"conn_timeout"`    // seconds
	KeepAlive     int               `json:"keep_alive"`      // seconds
	MaxStreams    int               `json:"max_streams"`
	BufferSize    int               `json:"buffer_size"`
	EnableReconnect bool            `json:"enable_reconnect"`
	ReconnectDelay  int             `json:"reconnect_delay"` // seconds
	MaxReconnects   int             `json:"max_reconnects"`
	DefaultQoS      QoSLevel        `json:"default_qos"`
	MessageTTL      int             `json:"message_ttl"`     // seconds
	Compression     bool            `json:"compression"`
	Debug           bool            `json:"debug"`
	LogLevel        string          `json:"log_level"`
	Metadata        map[string]string `json:"metadata"`
}

// TopicConfig represents topic configuration
type TopicConfig struct {
	Name        string            `json:"name"`
	Type        TopicType         `json:"type"`
	Persistent  bool              `json:"persistent"`
	Retention   int64             `json:"retention"`    // seconds
	MaxSize     int64             `json:"max_size"`     // bytes
	QoS         QoSLevel          `json:"qos"`
	Metadata    map[string]string `json:"metadata"`
}

// TopicInfo represents topic information
type TopicInfo struct {
	Config        *TopicConfig `json:"config"`
	Subscribers   int          `json:"subscribers"`
	MessageCount  int64        `json:"message_count"`
	TotalSize     int64        `json:"total_size"`
	CreatedAt     int64        `json:"created_at"`
	UpdatedAt     int64        `json:"updated_at"`
}

// ClientStats represents client statistics
type ClientStats struct {
	MessagesSent       int64 `json:"messages_sent"`
	MessagesReceived   int64 `json:"messages_received"`
	BytesSent          int64 `json:"bytes_sent"`
	BytesReceived      int64 `json:"bytes_received"`
	ActiveSubscriptions int   `json:"active_subscriptions"`
	ActiveStreams       int   `json:"active_streams"`
	ErrorCount         int64 `json:"error_count"`
	ConnectedAt        int64 `json:"connected_at"`
	LastActivity       int64 `json:"last_activity"`
}

// ConnectionInfo represents connection information
type ConnectionInfo struct {
	Status      string  `json:"status"`
	RemoteAddr  string  `json:"remote_addr"`
	LocalAddr   string  `json:"local_addr"`
	RTT         int64   `json:"rtt"`         // Round trip time in microseconds
	Bandwidth   string  `json:"bandwidth"`   // Estimated bandwidth
	PacketLoss  float64 `json:"packet_loss"` // Packet loss ratio
	ConnectedAt int64   `json:"connected_at"`
	Protocol    string  `json:"protocol"`
}

// StreamStats represents stream statistics
type StreamStats struct {
	StreamID      string `json:"stream_id"`
	BytesSent     int64  `json:"bytes_sent"`
	BytesReceived int64  `json:"bytes_received"`
	PacketsSent   int64  `json:"packets_sent"`
	PacketsReceived int64 `json:"packets_received"`
	StartTime     int64  `json:"start_time"`
	EndTime       int64  `json:"end_time,omitempty"`
	Status        string `json:"status"`
}

// Fragment represents a message fragment
type Fragment struct {
	MessageID       uint64 `json:"message_id"`
	FragmentIndex   int    `json:"fragment_index"`
	TotalFragments  int    `json:"total_fragments"`
	FragmentSize    int    `json:"fragment_size"`
	FragmentData    []byte `json:"fragment_data"`
	IsLast          bool   `json:"is_last"`
	IsCompressed    bool   `json:"is_compressed"`
	SerializeFormat uint8  `json:"serialize_format"`
	Checksum        string `json:"checksum"`
	Timestamp       int64  `json:"timestamp"`
}

// PendingMessage represents a message being reassembled
type PendingMessage struct {
	MessageID         uint64             `json:"message_id"`
	TotalFragments    int                `json:"total_fragments"`
	ReceivedFragments map[int]*Fragment  `json:"received_fragments"`
	FirstReceived     int64              `json:"first_received"`
	LastReceived      int64              `json:"last_received"`
	IsCompressed      bool               `json:"is_compressed"`
	SerializeFormat   uint8              `json:"serialize_format"`
}

// Error types
type SGCSFError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *SGCSFError) Error() string {
	if e.Details != "" {
		return e.Code + ": " + e.Message + " (" + e.Details + ")"
	}
	return e.Code + ": " + e.Message
}

// Common error codes
const (
	ErrTopicNotFound     = "TOPIC_NOT_FOUND"
	ErrInvalidMessage    = "INVALID_MESSAGE"
	ErrTimeout           = "TIMEOUT"
	ErrConnectionLost    = "CONNECTION_LOST"
	ErrSerialization     = "SERIALIZATION_ERROR"
	ErrFragmentation     = "FRAGMENTATION_ERROR"
	ErrPermissionDenied  = "PERMISSION_DENIED"
	ErrResourceExhausted = "RESOURCE_EXHAUSTED"
)

// NewSGCSFError creates a new SGCSF error
func NewSGCSFError(code, message string, details ...string) *SGCSFError {
	err := &SGCSFError{
		Code:    code,
		Message: message,
	}
	if len(details) > 0 {
		err.Details = details[0]
	}
	return err
}