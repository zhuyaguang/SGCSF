package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// MessageBuilder provides a fluent API for building SGCSF messages
type MessageBuilder struct {
	message *types.SGCSFMessage
}

// NewMessage creates a new message builder
func NewMessage() *MessageBuilder {
	return &MessageBuilder{
		message: types.NewMessage(),
	}
}

// Topic sets the message topic
func (b *MessageBuilder) Topic(topic string) *MessageBuilder {
	b.message.Topic = topic
	return b
}

// Type sets the message type
func (b *MessageBuilder) Type(msgType types.MessageType) *MessageBuilder {
	b.message.Type = msgType
	if msgType == types.MessageTypeSync {
		b.message.IsSync = true
		b.message.RequestID = types.GenerateRequestID()
	}
	return b
}

// Source sets the message source
func (b *MessageBuilder) Source(source string) *MessageBuilder {
	b.message.Source = source
	return b
}

// Destination sets the message destination
func (b *MessageBuilder) Destination(destination string) *MessageBuilder {
	b.message.Destination = destination
	return b
}

// Priority sets the message priority
func (b *MessageBuilder) Priority(priority types.Priority) *MessageBuilder {
	b.message.Priority = priority
	return b
}

// QoS sets the Quality of Service level
func (b *MessageBuilder) QoS(qos types.QoSLevel) *MessageBuilder {
	b.message.QoS = qos
	return b
}

// TTL sets the time-to-live for the message
func (b *MessageBuilder) TTL(ttl time.Duration) *MessageBuilder {
	b.message.SetTTL(ttl)
	return b
}

// Header sets a header value
func (b *MessageBuilder) Header(key, value string) *MessageBuilder {
	b.message.SetHeader(key, value)
	return b
}

// Metadata sets metadata
func (b *MessageBuilder) Metadata(key, value string) *MessageBuilder {
	b.message.SetMetadata(key, value)
	return b
}

// ContentType sets the content type
func (b *MessageBuilder) ContentType(contentType string) *MessageBuilder {
	b.message.ContentType = contentType
	return b
}

// ContentEncoding sets the content encoding
func (b *MessageBuilder) ContentEncoding(encoding string) *MessageBuilder {
	b.message.ContentEncoding = encoding
	return b
}

// Payload sets the message payload
func (b *MessageBuilder) Payload(payload interface{}) *MessageBuilder {
	switch v := payload.(type) {
	case []byte:
		b.message.Payload = v
	case string:
		b.message.Payload = []byte(v)
	default:
		// JSON marshal for complex types
		data, err := json.Marshal(v)
		if err == nil {
			b.message.Payload = data
			if b.message.ContentType == "" {
				b.message.ContentType = "application/json"
			}
		}
	}
	return b
}

// StreamID sets the stream ID for stream messages
func (b *MessageBuilder) StreamID(streamID string) *MessageBuilder {
	b.message.StreamID = streamID
	return b
}

// StreamType sets the stream type
func (b *MessageBuilder) StreamType(streamType types.StreamType) *MessageBuilder {
	b.message.StreamType = streamType
	return b
}

// StreamStart marks this as a stream start message
func (b *MessageBuilder) StreamStart() *MessageBuilder {
	b.message.IsStreamStart = true
	b.message.Type = types.MessageTypeStream
	return b
}

// StreamEnd marks this as a stream end message
func (b *MessageBuilder) StreamEnd() *MessageBuilder {
	b.message.IsStreamEnd = true
	b.message.Type = types.MessageTypeStream
	return b
}

// RequestID sets the request ID for sync messages
func (b *MessageBuilder) RequestID(requestID string) *MessageBuilder {
	b.message.RequestID = requestID
	return b
}

// ResponseTo sets the response target for response messages
func (b *MessageBuilder) ResponseTo(responseToID string) *MessageBuilder {
	b.message.ResponseTo = responseToID
	b.message.Type = types.MessageTypeResponse
	return b
}

// Timeout sets the timeout for sync messages
func (b *MessageBuilder) Timeout(timeout time.Duration) *MessageBuilder {
	b.message.Timeout = timeout.Milliseconds()
	return b
}

// Build returns the constructed message
func (b *MessageBuilder) Build() *types.SGCSFMessage {
	return b.message
}

// BuildJSON returns the message as JSON bytes
func (b *MessageBuilder) BuildJSON() ([]byte, error) {
	return json.Marshal(b.message)
}

// BuildCopy returns a copy of the constructed message
func (b *MessageBuilder) BuildCopy() *types.SGCSFMessage {
	return b.message.Clone()
}

// Validate validates the message before building
func (b *MessageBuilder) Validate() error {
	if b.message.Topic == "" {
		return fmt.Errorf("message topic is required")
	}
	
	if b.message.Source == "" {
		return fmt.Errorf("message source is required")
	}
	
	if b.message.IsSync && b.message.RequestID == "" {
		return fmt.Errorf("sync message requires request ID")
	}
	
	if b.message.Type == types.MessageTypeResponse && b.message.ResponseTo == "" {
		return fmt.Errorf("response message requires response target ID")
	}
	
	if b.message.Type == types.MessageTypeStream && b.message.StreamID == "" {
		return fmt.Errorf("stream message requires stream ID")
	}
	
	return nil
}

// Reset resets the builder to create a new message
func (b *MessageBuilder) Reset() *MessageBuilder {
	b.message = types.NewMessage()
	return b
}

// From creates a builder from an existing message
func FromMessage(message *types.SGCSFMessage) *MessageBuilder {
	return &MessageBuilder{
		message: message.Clone(),
	}
}

// QuickMessage creates a simple async message
func QuickMessage(topic string, payload interface{}) *types.SGCSFMessage {
	return NewMessage().
		Topic(topic).
		Type(types.MessageTypeAsync).
		Payload(payload).
		Build()
}

// QuickSyncMessage creates a simple sync message
func QuickSyncMessage(topic string, payload interface{}, timeout time.Duration) *types.SGCSFMessage {
	return NewMessage().
		Topic(topic).
		Type(types.MessageTypeSync).
		Payload(payload).
		Timeout(timeout).
		Build()
}

// QuickResponse creates a response message
func QuickResponse(originalMessage *types.SGCSFMessage, payload interface{}) *types.SGCSFMessage {
	return NewMessage().
		Type(types.MessageTypeResponse).
		Topic(originalMessage.Topic).
		ResponseTo(originalMessage.RequestID).
		Destination(originalMessage.Source).
		Payload(payload).
		Build()
}

// QuickBroadcast creates a broadcast message
func QuickBroadcast(topic string, payload interface{}) *types.SGCSFMessage {
	return NewMessage().
		Topic(topic).
		Type(types.MessageTypeBroadcast).
		Priority(types.PriorityHigh).
		Payload(payload).
		Build()
}

// ClientBuilder provides a fluent API for building SGCSF clients
type ClientBuilder struct {
	config *types.ClientConfig
}

// NewClientBuilder creates a new client builder
func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{
		config: &types.ClientConfig{
			ConnTimeout:     30,
			KeepAlive:       60,
			MaxStreams:      1000,
			BufferSize:      65536,
			EnableReconnect: true,
			ReconnectDelay:  5,
			MaxReconnects:   10,
			DefaultQoS:      types.QoSAtLeastOnce,
			MessageTTL:      300,
			Compression:     true,
			Debug:           false,
			LogLevel:        "info",
			Metadata:        make(map[string]string),
		},
	}
}

// ClientID sets the client ID
func (cb *ClientBuilder) ClientID(clientID string) *ClientBuilder {
	cb.config.ClientID = clientID
	return cb
}

// ServerAddr sets the server address
func (cb *ClientBuilder) ServerAddr(addr string) *ClientBuilder {
	cb.config.ServerAddr = addr
	return cb
}

// ConnTimeout sets the connection timeout
func (cb *ClientBuilder) ConnTimeout(timeout time.Duration) *ClientBuilder {
	cb.config.ConnTimeout = int(timeout.Seconds())
	return cb
}

// KeepAlive sets the keep-alive interval
func (cb *ClientBuilder) KeepAlive(interval time.Duration) *ClientBuilder {
	cb.config.KeepAlive = int(interval.Seconds())
	return cb
}

// MaxStreams sets the maximum number of streams
func (cb *ClientBuilder) MaxStreams(maxStreams int) *ClientBuilder {
	cb.config.MaxStreams = maxStreams
	return cb
}

// BufferSize sets the buffer size
func (cb *ClientBuilder) BufferSize(size int) *ClientBuilder {
	cb.config.BufferSize = size
	return cb
}

// EnableReconnect enables automatic reconnection
func (cb *ClientBuilder) EnableReconnect(enable bool) *ClientBuilder {
	cb.config.EnableReconnect = enable
	return cb
}

// ReconnectDelay sets the reconnection delay
func (cb *ClientBuilder) ReconnectDelay(delay time.Duration) *ClientBuilder {
	cb.config.ReconnectDelay = int(delay.Seconds())
	return cb
}

// MaxReconnects sets the maximum number of reconnection attempts
func (cb *ClientBuilder) MaxReconnects(max int) *ClientBuilder {
	cb.config.MaxReconnects = max
	return cb
}

// DefaultQoS sets the default QoS level
func (cb *ClientBuilder) DefaultQoS(qos types.QoSLevel) *ClientBuilder {
	cb.config.DefaultQoS = qos
	return cb
}

// MessageTTL sets the default message TTL
func (cb *ClientBuilder) MessageTTL(ttl time.Duration) *ClientBuilder {
	cb.config.MessageTTL = int(ttl.Seconds())
	return cb
}

// Compression enables or disables compression
func (cb *ClientBuilder) Compression(enable bool) *ClientBuilder {
	cb.config.Compression = enable
	return cb
}

// Debug enables or disables debug mode
func (cb *ClientBuilder) Debug(debug bool) *ClientBuilder {
	cb.config.Debug = debug
	return cb
}

// LogLevel sets the log level
func (cb *ClientBuilder) LogLevel(level string) *ClientBuilder {
	cb.config.LogLevel = level
	return cb
}

// Metadata sets client metadata
func (cb *ClientBuilder) Metadata(key, value string) *ClientBuilder {
	cb.config.Metadata[key] = value
	return cb
}

// Build creates a new SGCSF client
func (cb *ClientBuilder) Build() *Client {
	return NewClient(cb.config)
}

// BuildConfig returns the client configuration
func (cb *ClientBuilder) BuildConfig() *types.ClientConfig {
	return cb.config
}

// Validate validates the client configuration
func (cb *ClientBuilder) Validate() error {
	if cb.config.ClientID == "" {
		return fmt.Errorf("client ID is required")
	}
	
	if cb.config.ServerAddr == "" {
		return fmt.Errorf("server address is required")
	}
	
	if cb.config.ConnTimeout <= 0 {
		return fmt.Errorf("connection timeout must be positive")
	}
	
	if cb.config.KeepAlive <= 0 {
		return fmt.Errorf("keep-alive interval must be positive")
	}
	
	return nil
}

// Convenience functions for common client configurations

// QuickClient creates a client with basic configuration
func QuickClient(clientID, serverAddr string) *Client {
	return NewClientBuilder().
		ClientID(clientID).
		ServerAddr(serverAddr).
		Build()
}

// SatelliteClient creates a client optimized for satellite communication
func SatelliteClient(clientID, serverAddr string) *Client {
	return NewClientBuilder().
		ClientID(clientID).
		ServerAddr(serverAddr).
		ConnTimeout(30 * time.Second).
		KeepAlive(30 * time.Second).
		MaxStreams(100).
		BufferSize(32768).
		EnableReconnect(true).
		ReconnectDelay(10 * time.Second).
		MaxReconnects(5).
		DefaultQoS(types.QoSAtLeastOnce).
		MessageTTL(5 * time.Minute).
		Compression(true).
		Metadata("environment", "satellite").
		Build()
}

// GroundClient creates a client optimized for ground station communication
func GroundClient(clientID, serverAddr string) *Client {
	return NewClientBuilder().
		ClientID(clientID).
		ServerAddr(serverAddr).
		ConnTimeout(10 * time.Second).
		KeepAlive(60 * time.Second).
		MaxStreams(1000).
		BufferSize(65536).
		EnableReconnect(true).
		ReconnectDelay(5 * time.Second).
		MaxReconnects(10).
		DefaultQoS(types.QoSAtLeastOnce).
		MessageTTL(10 * time.Minute).
		Compression(false).
		Metadata("environment", "ground").
		Build()
}