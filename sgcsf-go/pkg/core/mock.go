package core

import (
	"fmt"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// MockClientConnection implements ClientConnection for testing
type MockClientConnection struct {
	clientID    string
	remoteAddr  string
	connected   bool
	connectedAt time.Time
	stats       *types.ClientStats
}

// NewMockClientConnection creates a new mock client connection
func NewMockClientConnection(clientID, remoteAddr string) *MockClientConnection {
	return &MockClientConnection{
		clientID:    clientID,
		remoteAddr:  remoteAddr,
		connected:   true,
		connectedAt: time.Now(),
		stats: &types.ClientStats{
			ConnectedAt: time.Now().Unix(),
		},
	}
}

// GetClientID returns the client ID
func (m *MockClientConnection) GetClientID() string {
	return m.clientID
}

// GetRemoteAddr returns the remote address
func (m *MockClientConnection) GetRemoteAddr() string {
	return m.remoteAddr
}

// IsConnected returns connection status
func (m *MockClientConnection) IsConnected() bool {
	return m.connected
}

// GetConnectedAt returns connection time
func (m *MockClientConnection) GetConnectedAt() time.Time {
	return m.connectedAt
}

// SendMessage sends a message (mock implementation)
func (m *MockClientConnection) SendMessage(message *types.SGCSFMessage) error {
	if !m.connected {
		return fmt.Errorf("connection closed")
	}
	
	m.stats.MessagesSent++
	m.stats.BytesSent += int64(len(message.Payload))
	
	// In a real implementation, this would send over QUIC
	fmt.Printf("MockConn: Sending message %s to topic %s\n", message.ID, message.Topic)
	return nil
}

// SendFragment sends a message fragment
func (m *MockClientConnection) SendFragment(fragment *types.Fragment) error {
	if !m.connected {
		return fmt.Errorf("connection closed")
	}
	
	// In a real implementation, this would send over QUIC
	fmt.Printf("MockConn: Sending fragment %d/%d for message %d\n", 
		fragment.FragmentIndex+1, fragment.TotalFragments, fragment.MessageID)
	return nil
}

// CreateStream creates a new stream (mock)
func (m *MockClientConnection) CreateStream(streamID string, streamType types.StreamType) (Stream, error) {
	return NewMockStream(streamID, streamType), nil
}

// AcceptStream accepts an incoming stream (mock)
func (m *MockClientConnection) AcceptStream(streamID string) (Stream, error) {
	return NewMockStream(streamID, types.StreamData), nil
}

// Close closes the connection
func (m *MockClientConnection) Close() error {
	m.connected = false
	return nil
}

// Ping sends a ping message
func (m *MockClientConnection) Ping() error {
	if !m.connected {
		return fmt.Errorf("connection closed")
	}
	return nil
}

// GetStats returns connection statistics
func (m *MockClientConnection) GetStats() (*types.ClientStats, error) {
	return m.stats, nil
}

// MockStream implements Stream interface for testing
type MockStream struct {
	streamID   string
	streamType types.StreamType
	closed     bool
	stats      *types.StreamStats
}

// NewMockStream creates a new mock stream
func NewMockStream(streamID string, streamType types.StreamType) *MockStream {
	return &MockStream{
		streamID:   streamID,
		streamType: streamType,
		stats: &types.StreamStats{
			StreamID:  streamID,
			StartTime: time.Now().Unix(),
			Status:    "active",
		},
	}
}

// Read reads data from the stream
func (m *MockStream) Read(p []byte) (int, error) {
	if m.closed {
		return 0, fmt.Errorf("stream closed")
	}
	// Mock implementation - would read from actual stream
	return 0, nil
}

// Write writes data to the stream
func (m *MockStream) Write(p []byte) (int, error) {
	if m.closed {
		return 0, fmt.Errorf("stream closed")
	}
	
	m.stats.BytesSent += int64(len(p))
	m.stats.PacketsSent++
	
	// Mock implementation - would write to actual stream
	return len(p), nil
}

// Close closes the stream
func (m *MockStream) Close() error {
	m.closed = true
	m.stats.EndTime = time.Now().Unix()
	m.stats.Status = "closed"
	return nil
}

// SetWriteDeadline sets write deadline
func (m *MockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets read deadline
func (m *MockStream) SetReadDeadline(t time.Time) error {
	return nil
}

// GetStreamID returns stream ID
func (m *MockStream) GetStreamID() string {
	return m.streamID
}

// GetStreamType returns stream type
func (m *MockStream) GetStreamType() types.StreamType {
	return m.streamType
}

// GetStreamStats returns stream statistics
func (m *MockStream) GetStreamStats() (*types.StreamStats, error) {
	return m.stats, nil
}

// Flush flushes the stream
func (m *MockStream) Flush() error {
	return nil
}

// CloseWrite closes write side of stream
func (m *MockStream) CloseWrite() error {
	return nil
}