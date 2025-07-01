package types

import (
	"time"
)

// MessageType defines the type of SGCSF message
type MessageType uint8

const (
	MessageTypeAsync     MessageType = iota // 异步消息
	MessageTypeSync                         // 同步消息
	MessageTypeResponse                     // 响应消息
	MessageTypeBroadcast                    // 广播消息
	MessageTypeStream                       // 流消息
)

// Priority defines message priority levels
type Priority uint8

const (
	PriorityLow Priority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// QoSLevel defines Quality of Service levels
type QoSLevel uint8

const (
	QoSAtMostOnce  QoSLevel = iota // 最多一次
	QoSAtLeastOnce                 // 至少一次
	QoSExactlyOnce                 // 恰好一次
)

// StreamType defines types of data streams
type StreamType uint8

const (
	StreamData StreamType = iota // 数据流
	StreamFile                   // 文件流
	StreamVideo                  // 视频流
	StreamAudio                  // 音频流
)

// SGCSFMessage represents the core message structure
type SGCSFMessage struct {
	// Basic fields
	ID        string    `json:"id"`        // 消息ID
	Topic     string    `json:"topic"`     // 主题
	Type      MessageType `json:"type"`    // 消息类型
	Source    string    `json:"source"`    // 消息源
	Destination string  `json:"destination,omitempty"` // 目标 (可选)
	Timestamp int64     `json:"timestamp"` // 时间戳

	// Message properties
	Priority Priority `json:"priority"` // 优先级
	QoS      QoSLevel `json:"qos"`      // QoS级别
	TTL      int64    `json:"ttl"`      // 生存时间(毫秒)

	// Sync message fields
	IsSync     bool   `json:"is_sync,omitempty"`     // 是否同步消息
	RequestID  string `json:"request_id,omitempty"`  // 请求ID
	ResponseTo string `json:"response_to,omitempty"` // 响应目标ID
	Timeout    int64  `json:"timeout,omitempty"`     // 超时时间(毫秒)

	// Stream fields
	StreamID      string     `json:"stream_id,omitempty"`      // 流ID
	StreamType    StreamType `json:"stream_type,omitempty"`    // 流类型
	IsStreamStart bool       `json:"is_stream_start,omitempty"` // 流开始标志
	IsStreamEnd   bool       `json:"is_stream_end,omitempty"`   // 流结束标志

	// Content
	ContentType     string `json:"content_type,omitempty"`     // 内容类型
	ContentEncoding string `json:"content_encoding,omitempty"` // 内容编码
	Payload         []byte `json:"payload"`                    // 消息载荷

	// Extended fields
	Headers  map[string]string `json:"headers,omitempty"`  // 消息头
	Metadata map[string]string `json:"metadata,omitempty"` // 元数据
}

// GetIDAsInt converts string ID to uint64 for binary serialization
func (m *SGCSFMessage) GetIDAsInt() uint64 {
	// Simple hash function for demo, should use proper hash in production
	hash := uint64(0)
	for _, c := range m.ID {
		hash = hash*31 + uint64(c)
	}
	return hash
}

// GetRequestIDAsInt converts request ID to uint64
func (m *SGCSFMessage) GetRequestIDAsInt() uint64 {
	if m.RequestID == "" {
		return 0
	}
	hash := uint64(0)
	for _, c := range m.RequestID {
		hash = hash*31 + uint64(c)
	}
	return hash
}

// GetResponseToAsInt converts response target ID to uint64
func (m *SGCSFMessage) GetResponseToAsInt() uint64 {
	if m.ResponseTo == "" {
		return 0
	}
	hash := uint64(0)
	for _, c := range m.ResponseTo {
		hash = hash*31 + uint64(c)
	}
	return hash
}

// NewMessage creates a new SGCSF message with default values
func NewMessage() *SGCSFMessage {
	return &SGCSFMessage{
		ID:        GenerateMessageID(),
		Timestamp: time.Now().UnixNano(),
		Type:      MessageTypeAsync,
		Priority:  PriorityNormal,
		QoS:       QoSAtLeastOnce,
		TTL:       300000, // 5 minutes default
		Headers:   make(map[string]string),
		Metadata:  make(map[string]string),
	}
}

// IsExpired checks if the message has expired
func (m *SGCSFMessage) IsExpired() bool {
	if m.TTL <= 0 {
		return false // No expiration
	}
	elapsed := time.Now().UnixNano() - m.Timestamp
	return elapsed > m.TTL*1000000 // Convert TTL from milliseconds to nanoseconds
}

// SetTTL sets the time-to-live for the message
func (m *SGCSFMessage) SetTTL(duration time.Duration) {
	m.TTL = duration.Milliseconds()
}

// SetHeader sets a header value
func (m *SGCSFMessage) SetHeader(key, value string) {
	if m.Headers == nil {
		m.Headers = make(map[string]string)
	}
	m.Headers[key] = value
}

// GetHeader gets a header value
func (m *SGCSFMessage) GetHeader(key string) (string, bool) {
	if m.Headers == nil {
		return "", false
	}
	value, exists := m.Headers[key]
	return value, exists
}

// SetMetadata sets metadata
func (m *SGCSFMessage) SetMetadata(key, value string) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]string)
	}
	m.Metadata[key] = value
}

// GetMetadata gets metadata
func (m *SGCSFMessage) GetMetadata(key string) (string, bool) {
	if m.Metadata == nil {
		return "", false
	}
	value, exists := m.Metadata[key]
	return value, exists
}

// Clone creates a deep copy of the message
func (m *SGCSFMessage) Clone() *SGCSFMessage {
	clone := &SGCSFMessage{
		ID:              m.ID,
		Topic:           m.Topic,
		Type:            m.Type,
		Source:          m.Source,
		Destination:     m.Destination,
		Timestamp:       m.Timestamp,
		Priority:        m.Priority,
		QoS:             m.QoS,
		TTL:             m.TTL,
		IsSync:          m.IsSync,
		RequestID:       m.RequestID,
		ResponseTo:      m.ResponseTo,
		Timeout:         m.Timeout,
		StreamID:        m.StreamID,
		StreamType:      m.StreamType,
		IsStreamStart:   m.IsStreamStart,
		IsStreamEnd:     m.IsStreamEnd,
		ContentType:     m.ContentType,
		ContentEncoding: m.ContentEncoding,
		Headers:         make(map[string]string),
		Metadata:        make(map[string]string),
	}

	// Deep copy payload
	if len(m.Payload) > 0 {
		clone.Payload = make([]byte, len(m.Payload))
		copy(clone.Payload, m.Payload)
	}

	// Deep copy headers
	for k, v := range m.Headers {
		clone.Headers[k] = v
	}

	// Deep copy metadata
	for k, v := range m.Metadata {
		clone.Metadata[k] = v
	}

	return clone
}