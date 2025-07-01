package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// messageRouter implements MessageRouter interface
type messageRouter struct {
	client       *Client
	syncHandlers map[string]SyncHandler
	mutex        sync.RWMutex
}

// NewMessageRouter creates a new message router
func NewMessageRouter(client *Client) MessageRouter {
	return &messageRouter{
		client:       client,
		syncHandlers: make(map[string]SyncHandler),
	}
}

// RouteMessage routes a message to appropriate handlers
func (mr *messageRouter) RouteMessage(message *types.SGCSFMessage) error {
	switch message.Type {
	case types.MessageTypeAsync:
		return mr.routeAsyncMessage(message)
	case types.MessageTypeSync:
		return mr.routeSyncMessage(message)
	case types.MessageTypeResponse:
		return mr.routeResponseMessage(message)
	case types.MessageTypeBroadcast:
		return mr.routeBroadcastMessage(message)
	case types.MessageTypeStream:
		return mr.RouteStreamMessage(message)
	default:
		return fmt.Errorf("unknown message type: %d", message.Type)
	}
}

// RouteFragment routes a message fragment
func (mr *messageRouter) RouteFragment(fragment *types.Fragment) error {
	// Reassemble fragment into complete message
	message, err := mr.client.reassembler.ProcessFragment(fragment)
	if err != nil {
		return fmt.Errorf("fragment processing failed: %v", err)
	}

	// If message is complete, route it
	if message != nil {
		return mr.RouteMessage(message)
	}

	// Fragment was stored, waiting for more fragments
	return nil
}

// RegisterSyncHandler registers a handler for sync responses
func (mr *messageRouter) RegisterSyncHandler(requestID string, handler SyncHandler) error {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	mr.syncHandlers[requestID] = handler
	return nil
}

// HandleSyncResponse handles a sync response
func (mr *messageRouter) HandleSyncResponse(response *types.SGCSFMessage) error {
	mr.mutex.RLock()
	handler, exists := mr.syncHandlers[response.ResponseTo]
	mr.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("no handler found for sync response: %s", response.ResponseTo)
	}

	return handler.HandleResponse(response)
}

// RouteStreamMessage routes stream messages
func (mr *messageRouter) RouteStreamMessage(message *types.SGCSFMessage) error {
	if message.StreamID == "" {
		return fmt.Errorf("stream message missing stream ID")
	}

	// Route to stream manager
	return mr.client.streamManager.RouteStreamData(message.StreamID, message.Payload)
}

// routeAsyncMessage routes async messages to subscribers
func (mr *messageRouter) routeAsyncMessage(message *types.SGCSFMessage) error {
	subscribers, err := mr.client.topicManager.RouteMessage(message)
	if err != nil {
		return err
	}

	// Deliver to all subscribers
	for _, subscription := range subscribers {
		go func(sub *types.Subscription) {
			if sub.Handler != nil {
				err := sub.Handler.Handle(message)
				if err != nil {
					// Log error but don't fail the entire routing
					fmt.Printf("Handler error for subscription %s: %v\n", sub.ID, err)
				}
			}
		}(subscription)
	}

	return nil
}

// routeSyncMessage routes sync messages and expects responses
func (mr *messageRouter) routeSyncMessage(message *types.SGCSFMessage) error {
	subscribers, err := mr.client.topicManager.RouteMessage(message)
	if err != nil {
		return err
	}

	if len(subscribers) == 0 {
		return fmt.Errorf("no subscribers found for topic: %s", message.Topic)
	}

	// For sync messages, we typically send to the first available handler
	// In a real implementation, you might have more sophisticated routing
	subscription := subscribers[0]
	
	if subscription.Handler != nil {
		return subscription.Handler.Handle(message)
	}

	return fmt.Errorf("no handler available for sync message")
}

// routeResponseMessage routes response messages to sync handlers
func (mr *messageRouter) routeResponseMessage(message *types.SGCSFMessage) error {
	// Handle response in client
	mr.client.handleSyncResponse(message)
	return nil
}

// routeBroadcastMessage routes broadcast messages to all subscribers
func (mr *messageRouter) routeBroadcastMessage(message *types.SGCSFMessage) error {
	// Same as async for now, but could have different behavior
	return mr.routeAsyncMessage(message)
}

// streamManager implements StreamManager interface
type streamManager struct {
	streams  map[string]Stream
	handlers map[types.StreamType]StreamHandler
	mutex    sync.RWMutex
}

// NewStreamManager creates a new stream manager
func NewStreamManager() StreamManager {
	return &streamManager{
		streams:  make(map[string]Stream),
		handlers: make(map[types.StreamType]StreamHandler),
	}
}

// CreateStream creates a new stream
func (sm *streamManager) CreateStream(streamID string, streamType types.StreamType, clientConn ClientConnection) (Stream, error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, exists := sm.streams[streamID]; exists {
		return nil, fmt.Errorf("stream already exists: %s", streamID)
	}

	stream, err := clientConn.CreateStream(streamID, streamType)
	if err != nil {
		return nil, err
	}

	sm.streams[streamID] = stream

	// Notify handler if registered
	if handler, exists := sm.handlers[streamType]; exists {
		go handler.OnStreamCreated(stream)
	}

	return stream, nil
}

// GetStream retrieves a stream
func (sm *streamManager) GetStream(streamID string) (Stream, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stream, exists := sm.streams[streamID]
	if !exists {
		return nil, fmt.Errorf("stream not found: %s", streamID)
	}

	return stream, nil
}

// CloseStream closes a stream
func (sm *streamManager) CloseStream(streamID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stream, exists := sm.streams[streamID]
	if !exists {
		return fmt.Errorf("stream not found: %s", streamID)
	}

	err := stream.Close()
	delete(sm.streams, streamID)

	// Notify handler if registered
	if handler, exists := sm.handlers[stream.GetStreamType()]; exists {
		go handler.OnStreamClosed(stream)
	}

	return err
}

// ListStreams lists all active streams
func (sm *streamManager) ListStreams() []string {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	streamIDs := make([]string, 0, len(sm.streams))
	for streamID := range sm.streams {
		streamIDs = append(streamIDs, streamID)
	}

	return streamIDs
}

// RegisterStreamHandler registers a handler for stream events
func (sm *streamManager) RegisterStreamHandler(streamType types.StreamType, handler StreamHandler) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.handlers[streamType] = handler
	return nil
}

// RouteStreamData routes data to a specific stream
func (sm *streamManager) RouteStreamData(streamID string, data []byte) error {
	stream, err := sm.GetStream(streamID)
	if err != nil {
		return err
	}

	// Notify handler if registered
	if handler, exists := sm.handlers[stream.GetStreamType()]; exists {
		return handler.OnStreamData(stream, data)
	}

	return nil
}

// qosManager implements QoSManager interface
type qosManager struct {
	pendingMessages map[string]*types.SGCSFMessage
	mutex           sync.RWMutex
}

// NewQoSManager creates a new QoS manager
func NewQoSManager() QoSManager {
	return &qosManager{
		pendingMessages: make(map[string]*types.SGCSFMessage),
	}
}

// HandleMessage handles message according to QoS level
func (qm *qosManager) HandleMessage(message *types.SGCSFMessage) error {
	switch message.QoS {
	case types.QoSAtMostOnce:
		// Fire and forget
		return nil
	case types.QoSAtLeastOnce:
		// Store for potential retry
		return qm.EnsureDelivery(message, message.QoS)
	case types.QoSExactlyOnce:
		// Ensure exactly once delivery
		return qm.EnsureDelivery(message, message.QoS)
	default:
		return fmt.Errorf("unsupported QoS level: %d", message.QoS)
	}
}

// EnsureDelivery ensures message delivery according to QoS
func (qm *qosManager) EnsureDelivery(message *types.SGCSFMessage, qos types.QoSLevel) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	// Store message for potential retry
	qm.pendingMessages[message.ID] = message

	// Set TTL if not already set
	if message.TTL == 0 {
		message.SetTTL(5 * time.Minute) // Default 5 minutes
	}

	return nil
}

// AcknowledgeMessage acknowledges message delivery
func (qm *qosManager) AcknowledgeMessage(messageID string) error {
	qm.mutex.Lock()
	defer qm.mutex.Unlock()

	delete(qm.pendingMessages, messageID)
	return nil
}

// PrioritizeMessage returns priority score for message scheduling
func (qm *qosManager) PrioritizeMessage(message *types.SGCSFMessage) int {
	score := int(message.Priority) * 100

	// Add urgency based on TTL
	if message.TTL > 0 {
		elapsed := time.Now().UnixNano() - message.Timestamp
		remaining := message.TTL*1000000 - elapsed
		if remaining < message.TTL*1000000/2 { // Less than half TTL remaining
			score += 50
		}
	}

	return score
}

// ScheduleMessage schedules message for transmission
func (qm *qosManager) ScheduleMessage(message *types.SGCSFMessage) error {
	// In a real implementation, this would add to a priority queue
	// For now, just return success
	return nil
}