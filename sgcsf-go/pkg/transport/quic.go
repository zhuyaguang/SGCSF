package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
	"github.com/sgcsf/sgcsf-go/pkg/serialization"
)

// QUICTransport implements QUIC-based transport for SGCSF
type QUICTransport struct {
	config       *QUICConfig
	listener     net.Listener
	connections  map[string]*QUICConnection
	serializer   serialization.MessageSerializer
	fragmenter   *serialization.MessageFragmenter
	reassembler  *serialization.MessageReassembler
	mutex        sync.RWMutex
	running      bool
	stopChan     chan struct{}
}

// QUICConfig represents QUIC transport configuration
type QUICConfig struct {
	ListenAddr      string        `json:"listen_addr"`
	TLSConfig       *tls.Config   `json:"-"`
	MaxStreams      int           `json:"max_streams"`
	KeepAlive       time.Duration `json:"keep_alive"`
	HandshakeTimeout time.Duration `json:"handshake_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	MaxPacketSize   int           `json:"max_packet_size"`
	EnableRetries   bool          `json:"enable_retries"`
	MaxRetries      int           `json:"max_retries"`
}

// QUICConnection represents a QUIC connection
type QUICConnection struct {
	id          string
	remoteAddr  net.Addr
	localAddr   net.Addr
	conn        net.Conn // Simplified for demo - would be quic.Connection in real implementation
	streams     map[string]*QUICStream
	transport   *QUICTransport
	stats       *ConnectionStats
	lastActivity time.Time
	mutex       sync.RWMutex
	closed      bool
}

// QUICStream represents a QUIC stream
type QUICStream struct {
	id         string
	streamType types.StreamType
	conn       *QUICConnection
	readBuf    []byte
	writeBuf   []byte
	closed     bool
	stats      *types.StreamStats
	mutex      sync.RWMutex
}

// ConnectionStats represents connection statistics
type ConnectionStats struct {
	PacketsSent     int64
	PacketsReceived int64
	BytesSent       int64
	BytesReceived   int64
	RTT             time.Duration
	PacketLoss      float64
	ConnectedAt     time.Time
	LastActivity    time.Time
}

// NewQUICTransport creates a new QUIC transport
func NewQUICTransport(config *QUICConfig) *QUICTransport {
	if config == nil {
		config = &QUICConfig{
			ListenAddr:       ":7000",
			MaxStreams:       1000,
			KeepAlive:        30 * time.Second,
			HandshakeTimeout: 10 * time.Second,
			IdleTimeout:      5 * time.Minute,
			MaxPacketSize:    1350, // Standard MTU minus headers
			EnableRetries:    true,
			MaxRetries:       3,
		}
	}

	if config.TLSConfig == nil {
		config.TLSConfig = generateTLSConfig()
	}

	qt := &QUICTransport{
		config:      config,
		connections: make(map[string]*QUICConnection),
		serializer:  serialization.NewBinarySerializer(),
		fragmenter:  serialization.NewMessageFragmenter(),
		reassembler: serialization.NewMessageReassembler(),
		stopChan:    make(chan struct{}),
	}

	return qt
}

// Listen starts listening for QUIC connections
func (qt *QUICTransport) Listen(ctx context.Context) error {
	qt.mutex.Lock()
	defer qt.mutex.Unlock()

	if qt.running {
		return fmt.Errorf("transport already running")
	}

	// Create listener (simplified - would use quic.Listen in real implementation)
	listener, err := net.Listen("tcp", qt.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %v", err)
	}

	qt.listener = listener
	qt.running = true

	// Start accepting connections
	go qt.acceptLoop(ctx)

	fmt.Printf("QUIC transport listening on %s\n", qt.config.ListenAddr)
	return nil
}

// Stop stops the QUIC transport
func (qt *QUICTransport) Stop() error {
	qt.mutex.Lock()
	defer qt.mutex.Unlock()

	if !qt.running {
		return nil
	}

	close(qt.stopChan)
	qt.running = false

	// Close listener
	if qt.listener != nil {
		qt.listener.Close()
	}

	// Close all connections
	for _, conn := range qt.connections {
		conn.Close()
	}

	return nil
}

// Connect establishes a QUIC connection to a remote address
func (qt *QUICTransport) Connect(ctx context.Context, addr string) (*QUICConnection, error) {
	// Create connection (simplified - would use quic.Dial in real implementation)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	qconn := &QUICConnection{
		id:         types.GenerateClientID("conn"),
		remoteAddr: conn.RemoteAddr(),
		localAddr:  conn.LocalAddr(),
		conn:       conn,
		streams:    make(map[string]*QUICStream),
		transport:  qt,
		stats: &ConnectionStats{
			ConnectedAt:  time.Now(),
			LastActivity: time.Now(),
		},
		lastActivity: time.Now(),
	}

	qt.mutex.Lock()
	qt.connections[qconn.id] = qconn
	qt.mutex.Unlock()

	// Start connection handler
	go qconn.handleConnection(ctx)

	return qconn, nil
}

// GetConnection retrieves a connection by ID
func (qt *QUICTransport) GetConnection(connID string) (*QUICConnection, error) {
	qt.mutex.RLock()
	defer qt.mutex.RUnlock()

	conn, exists := qt.connections[connID]
	if !exists {
		return nil, fmt.Errorf("connection not found: %s", connID)
	}

	return conn, nil
}

// SendMessage sends a message over QUIC
func (qt *QUICTransport) SendMessage(connID string, message *types.SGCSFMessage) error {
	conn, err := qt.GetConnection(connID)
	if err != nil {
		return err
	}

	return conn.SendMessage(message)
}

// BroadcastMessage sends a message to all connections
func (qt *QUICTransport) BroadcastMessage(message *types.SGCSFMessage) error {
	qt.mutex.RLock()
	connections := make([]*QUICConnection, 0, len(qt.connections))
	for _, conn := range qt.connections {
		connections = append(connections, conn)
	}
	qt.mutex.RUnlock()

	for _, conn := range connections {
		go conn.SendMessage(message)
	}

	return nil
}

// acceptLoop accepts incoming connections
func (qt *QUICTransport) acceptLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-qt.stopChan:
			return
		default:
			conn, err := qt.listener.Accept()
			if err != nil {
				if qt.running {
					fmt.Printf("Accept error: %v\n", err)
				}
				continue
			}

			// Handle new connection
			go qt.handleNewConnection(ctx, conn)
		}
	}
}

// handleNewConnection handles a new incoming connection
func (qt *QUICTransport) handleNewConnection(ctx context.Context, conn net.Conn) {
	qconn := &QUICConnection{
		id:         types.GenerateClientID("conn"),
		remoteAddr: conn.RemoteAddr(),
		localAddr:  conn.LocalAddr(),
		conn:       conn,
		streams:    make(map[string]*QUICStream),
		transport:  qt,
		stats: &ConnectionStats{
			ConnectedAt:  time.Now(),
			LastActivity: time.Now(),
		},
		lastActivity: time.Now(),
	}

	qt.mutex.Lock()
	qt.connections[qconn.id] = qconn
	qt.mutex.Unlock()

	fmt.Printf("New QUIC connection: %s from %s\n", qconn.id, qconn.remoteAddr)

	// Handle connection
	qconn.handleConnection(ctx)

	// Cleanup on disconnect
	qt.mutex.Lock()
	delete(qt.connections, qconn.id)
	qt.mutex.Unlock()
}

// Connection methods

// SendMessage sends a message over the connection
func (qc *QUICConnection) SendMessage(message *types.SGCSFMessage) error {
	if qc.closed {
		return fmt.Errorf("connection closed")
	}

	// Fragment message if needed
	fragments, err := qc.transport.fragmenter.Fragment(message)
	if err != nil {
		return fmt.Errorf("fragmentation failed: %v", err)
	}

	// Send each fragment
	for _, fragment := range fragments {
		err := qc.SendFragment(fragment)
		if err != nil {
			return fmt.Errorf("failed to send fragment: %v", err)
		}
	}

	qc.stats.PacketsSent += int64(len(fragments))
	qc.stats.BytesSent += int64(len(message.Payload))
	qc.lastActivity = time.Now()

	return nil
}

// SendFragment sends a message fragment
func (qc *QUICConnection) SendFragment(fragment *types.Fragment) error {
	if qc.closed {
		return fmt.Errorf("connection closed")
	}

	// Convert fragment to binary
	data := serialization.FragmentToBinary(fragment)

	// Send over connection (simplified)
	_, err := qc.conn.Write(data)
	if err != nil {
		return fmt.Errorf("write failed: %v", err)
	}

	return nil
}

// CreateStream creates a new stream on the connection
func (qc *QUICConnection) CreateStream(streamID string, streamType types.StreamType) (*QUICStream, error) {
	qc.mutex.Lock()
	defer qc.mutex.Unlock()

	if qc.closed {
		return nil, fmt.Errorf("connection closed")
	}

	if _, exists := qc.streams[streamID]; exists {
		return nil, fmt.Errorf("stream already exists: %s", streamID)
	}

	stream := &QUICStream{
		id:         streamID,
		streamType: streamType,
		conn:       qc,
		readBuf:    make([]byte, 0),
		writeBuf:   make([]byte, 0),
		stats: &types.StreamStats{
			StreamID:  streamID,
			StartTime: time.Now().Unix(),
			Status:    "active",
		},
	}

	qc.streams[streamID] = stream
	return stream, nil
}

// GetStream retrieves a stream by ID
func (qc *QUICConnection) GetStream(streamID string) (*QUICStream, error) {
	qc.mutex.RLock()
	defer qc.mutex.RUnlock()

	stream, exists := qc.streams[streamID]
	if !exists {
		return nil, fmt.Errorf("stream not found: %s", streamID)
	}

	return stream, nil
}

// Close closes the connection
func (qc *QUICConnection) Close() error {
	qc.mutex.Lock()
	defer qc.mutex.Unlock()

	if qc.closed {
		return nil
	}

	qc.closed = true

	// Close all streams
	for _, stream := range qc.streams {
		stream.Close()
	}

	// Close underlying connection
	if qc.conn != nil {
		qc.conn.Close()
	}

	return nil
}

// GetStats returns connection statistics
func (qc *QUICConnection) GetStats() *ConnectionStats {
	qc.mutex.RLock()
	defer qc.mutex.RUnlock()

	// Create a copy
	stats := *qc.stats
	stats.LastActivity = qc.lastActivity
	return &stats
}

// GetRemoteAddr returns the remote address
func (qc *QUICConnection) GetRemoteAddr() string {
	return qc.remoteAddr.String()
}

// handleConnection handles connection events
func (qc *QUICConnection) handleConnection(ctx context.Context) {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Set read timeout
			qc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

			// Read data
			n, err := qc.conn.Read(buffer)
			if err != nil {
				if !qc.closed {
					fmt.Printf("Connection read error: %v\n", err)
				}
				return
			}

			qc.lastActivity = time.Now()
			qc.stats.PacketsReceived++
			qc.stats.BytesReceived += int64(n)

			// Process received data (simplified)
			go qc.processReceivedData(buffer[:n])
		}
	}
}

// processReceivedData processes received data
func (qc *QUICConnection) processReceivedData(data []byte) {
	// In a real implementation, this would:
	// 1. Parse the fragment from binary data
	// 2. Reassemble fragments into complete messages
	// 3. Route messages to appropriate handlers
	fmt.Printf("Received %d bytes from %s\n", len(data), qc.remoteAddr)
}

// Stream methods

// Read reads data from the stream
func (qs *QUICStream) Read(p []byte) (int, error) {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	if qs.closed {
		return 0, fmt.Errorf("stream closed")
	}

	// Read from buffer (simplified)
	n := copy(p, qs.readBuf)
	qs.readBuf = qs.readBuf[n:]

	qs.stats.BytesReceived += int64(n)
	qs.stats.PacketsReceived++

	return n, nil
}

// Write writes data to the stream
func (qs *QUICStream) Write(p []byte) (int, error) {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	if qs.closed {
		return 0, fmt.Errorf("stream closed")
	}

	// Write to buffer (simplified)
	qs.writeBuf = append(qs.writeBuf, p...)

	qs.stats.BytesSent += int64(len(p))
	qs.stats.PacketsSent++

	return len(p), nil
}

// Close closes the stream
func (qs *QUICStream) Close() error {
	qs.mutex.Lock()
	defer qs.mutex.Unlock()

	if qs.closed {
		return nil
	}

	qs.closed = true
	qs.stats.EndTime = time.Now().Unix()
	qs.stats.Status = "closed"

	// Remove from connection
	qs.conn.mutex.Lock()
	delete(qs.conn.streams, qs.id)
	qs.conn.mutex.Unlock()

	return nil
}

// SetWriteDeadline sets write deadline
func (qs *QUICStream) SetWriteDeadline(t time.Time) error {
	return nil // Simplified
}

// SetReadDeadline sets read deadline
func (qs *QUICStream) SetReadDeadline(t time.Time) error {
	return nil // Simplified
}

// GetStreamID returns the stream ID
func (qs *QUICStream) GetStreamID() string {
	return qs.id
}

// GetStreamType returns the stream type
func (qs *QUICStream) GetStreamType() types.StreamType {
	return qs.streamType
}

// GetStreamStats returns stream statistics
func (qs *QUICStream) GetStreamStats() (*types.StreamStats, error) {
	qs.mutex.RLock()
	defer qs.mutex.RUnlock()

	// Create a copy
	stats := *qs.stats
	return &stats, nil
}

// Flush flushes the stream
func (qs *QUICStream) Flush() error {
	// Send buffered data (simplified)
	return nil
}

// CloseWrite closes the write side of the stream
func (qs *QUICStream) CloseWrite() error {
	// Close write side (simplified)
	return nil
}

// generateTLSConfig generates a basic TLS configuration for testing
func generateTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // Only for testing
		NextProtos:         []string{"sgcsf"},
	}
}

// GetConnections returns all active connections
func (qt *QUICTransport) GetConnections() map[string]*QUICConnection {
	qt.mutex.RLock()
	defer qt.mutex.RUnlock()

	connections := make(map[string]*QUICConnection)
	for id, conn := range qt.connections {
		connections[id] = conn
	}

	return connections
}

// GetConnectionCount returns the number of active connections
func (qt *QUICTransport) GetConnectionCount() int {
	qt.mutex.RLock()
	defer qt.mutex.RUnlock()

	return len(qt.connections)
}