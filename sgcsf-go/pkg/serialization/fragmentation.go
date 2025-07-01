package serialization

import (
	"fmt"
	"sync"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// MessageFragmenter handles message fragmentation for MTU compliance
type MessageFragmenter struct {
	maxFragmentSize int                   // Maximum fragment size (750 bytes for MTU=800)
	compressor      MessageCompressor     // Optional compressor
	checksummer     Checksummer          // Checksum calculator
	serializer      MessageSerializer    // Serializer to use
}

// NewMessageFragmenter creates a new message fragmenter
func NewMessageFragmenter() *MessageFragmenter {
	return &MessageFragmenter{
		maxFragmentSize: 750,               // MTU 800 - 50 bytes for headers
		compressor:      NewLZ4Compressor(),
		checksummer:     NewCRC32Checksummer(),
		serializer:      NewBinarySerializer(),
	}
}

// Fragment fragments a message into MTU-compliant pieces
func (mf *MessageFragmenter) Fragment(message *types.SGCSFMessage) ([]*types.Fragment, error) {
	// 1. Serialize the message
	serialized, err := mf.serializer.Serialize(message)
	if err != nil {
		return nil, fmt.Errorf("serialization failed: %v", err)
	}

	// 2. Try compression if message is large enough
	data := serialized
	isCompressed := false
	if len(serialized) > 200 && mf.compressor != nil {
		compressed, err := mf.compressor.Compress(serialized)
		if err == nil && len(compressed) < len(serialized) {
			data = compressed
			isCompressed = true
		}
	}

	// 3. Check if fragmentation is needed
	if len(data) <= mf.maxFragmentSize {
		// Single fragment
		fragment := &types.Fragment{
			MessageID:       message.GetIDAsInt(),
			FragmentIndex:   0,
			TotalFragments:  1,
			FragmentSize:    len(data),
			FragmentData:    data,
			IsLast:          true,
			IsCompressed:    isCompressed,
			SerializeFormat: uint8(mf.serializer.GetFormat()),
			Checksum:        mf.checksummer.Calculate(data),
			Timestamp:       time.Now().UnixNano(),
		}
		return []*types.Fragment{fragment}, nil
	}

	// 4. Create multiple fragments
	fragments := make([]*types.Fragment, 0)
	messageID := message.GetIDAsInt()
	totalFragments := (len(data) + mf.maxFragmentSize - 1) / mf.maxFragmentSize

	for i := 0; i < totalFragments; i++ {
		start := i * mf.maxFragmentSize
		end := start + mf.maxFragmentSize
		if end > len(data) {
			end = len(data)
		}

		fragmentData := data[start:end]
		fragment := &types.Fragment{
			MessageID:       messageID,
			FragmentIndex:   i,
			TotalFragments:  totalFragments,
			FragmentSize:    len(fragmentData),
			FragmentData:    fragmentData,
			IsLast:          i == totalFragments-1,
			IsCompressed:    isCompressed,
			SerializeFormat: uint8(mf.serializer.GetFormat()),
			Checksum:        mf.checksummer.Calculate(fragmentData),
			Timestamp:       time.Now().UnixNano(),
		}

		fragments = append(fragments, fragment)
	}

	return fragments, nil
}

// FragmentToBinary converts a fragment to binary format for transmission
func FragmentToBinary(f *types.Fragment) []byte {
	// Use binary serializer to convert fragment to bytes
	serializer := NewBinarySerializer()
	
	// Create a temporary message to hold fragment data
	tempMessage := &types.SGCSFMessage{
		ID:        fmt.Sprintf("frag_%d_%d", f.MessageID, f.FragmentIndex),
		Topic:     "__fragment__",
		Type:      types.MessageTypeAsync,
		Timestamp: f.Timestamp,
		Payload:   f.FragmentData,
		Headers: map[string]string{
			"fragment_index":   fmt.Sprintf("%d", f.FragmentIndex),
			"total_fragments":  fmt.Sprintf("%d", f.TotalFragments),
			"message_id":       fmt.Sprintf("%d", f.MessageID),
			"checksum":         f.Checksum,
			"is_compressed":    fmt.Sprintf("%t", f.IsCompressed),
			"serialize_format": fmt.Sprintf("%d", f.SerializeFormat),
			"is_last":          fmt.Sprintf("%t", f.IsLast),
		},
	}

	data, _ := serializer.Serialize(tempMessage)
	return data
}

// MessageReassembler handles message reassembly from fragments
type MessageReassembler struct {
	pendingMessages map[uint64]*types.PendingMessage
	timeout         time.Duration
	maxPending      int
	mutex           sync.RWMutex
	compressors     map[CompressionType]MessageCompressor
	serializers     map[SerializationFormat]MessageSerializer
}

// NewMessageReassembler creates a new message reassembler
func NewMessageReassembler() *MessageReassembler {
	mr := &MessageReassembler{
		pendingMessages: make(map[uint64]*types.PendingMessage),
		timeout:         30 * time.Second,
		maxPending:      1000,
		compressors:     make(map[CompressionType]MessageCompressor),
		serializers:     make(map[SerializationFormat]MessageSerializer),
	}

	// Register compressors and serializers
	mr.compressors[CompressionLZ4] = NewLZ4Compressor()
	mr.compressors[CompressionGzip] = NewGzipCompressor()
	mr.serializers[FormatBinary] = NewBinarySerializer()
	mr.serializers[FormatJSON] = NewJSONSerializer()

	// Start cleanup routine
	go mr.cleanupRoutine()

	return mr
}

// ProcessFragment processes a fragment and returns a complete message if ready
func (mr *MessageReassembler) ProcessFragment(fragment *types.Fragment) (*types.SGCSFMessage, error) {
	mr.mutex.Lock()
	defer mr.mutex.Unlock()

	messageID := fragment.MessageID

	// Get or create pending message
	pending, exists := mr.pendingMessages[messageID]
	if !exists {
		// Check pending messages limit
		if len(mr.pendingMessages) >= mr.maxPending {
			return nil, fmt.Errorf("too many pending messages")
		}

		pending = &types.PendingMessage{
			MessageID:         messageID,
			TotalFragments:    fragment.TotalFragments,
			ReceivedFragments: make(map[int]*types.Fragment),
			FirstReceived:     time.Now().UnixNano(),
			IsCompressed:      fragment.IsCompressed,
			SerializeFormat:   fragment.SerializeFormat,
		}
		mr.pendingMessages[messageID] = pending
	}

	// Validate fragment consistency
	if err := mr.validateFragment(fragment, pending); err != nil {
		return nil, err
	}

	// Add fragment
	pending.ReceivedFragments[fragment.FragmentIndex] = fragment
	pending.LastReceived = time.Now().UnixNano()

	// Check if all fragments received
	if len(pending.ReceivedFragments) == pending.TotalFragments {
		// Reassemble message
		message, err := mr.reassembleMessage(pending)
		if err != nil {
			return nil, err
		}

		// Cleanup
		delete(mr.pendingMessages, messageID)

		return message, nil
	}

	return nil, nil // Still waiting for more fragments
}

// validateFragment validates fragment consistency
func (mr *MessageReassembler) validateFragment(fragment *types.Fragment, pending *types.PendingMessage) error {
	if fragment.TotalFragments != pending.TotalFragments {
		return fmt.Errorf("fragment total count mismatch: expected %d, got %d",
			pending.TotalFragments, fragment.TotalFragments)
	}

	if fragment.FragmentIndex >= fragment.TotalFragments {
		return fmt.Errorf("invalid fragment index: %d >= %d",
			fragment.FragmentIndex, fragment.TotalFragments)
	}

	if _, exists := pending.ReceivedFragments[fragment.FragmentIndex]; exists {
		return fmt.Errorf("duplicate fragment: %d", fragment.FragmentIndex)
	}

	// Verify checksum
	checksummer := NewCRC32Checksummer()
	if !checksummer.Verify(fragment.FragmentData, fragment.Checksum) {
		return fmt.Errorf("fragment checksum verification failed")
	}

	return nil
}

// reassembleMessage reassembles a complete message from fragments
func (mr *MessageReassembler) reassembleMessage(pending *types.PendingMessage) (*types.SGCSFMessage, error) {
	// 1. Sort fragments by index
	fragments := make([]*types.Fragment, pending.TotalFragments)
	totalSize := 0

	for i := 0; i < pending.TotalFragments; i++ {
		fragment, exists := pending.ReceivedFragments[i]
		if !exists {
			return nil, fmt.Errorf("missing fragment %d", i)
		}
		fragments[i] = fragment
		totalSize += fragment.FragmentSize
	}

	// 2. Combine fragment data
	data := make([]byte, 0, totalSize)
	for _, fragment := range fragments {
		data = append(data, fragment.FragmentData...)
	}

	// 3. Decompress if needed
	if pending.IsCompressed {
		compressor, exists := mr.compressors[CompressionLZ4] // Default to LZ4
		if !exists {
			return nil, fmt.Errorf("compressor not available")
		}

		decompressed, err := compressor.Decompress(data)
		if err != nil {
			return nil, fmt.Errorf("decompression failed: %v", err)
		}
		data = decompressed
	}

	// 4. Deserialize message
	serializer, exists := mr.serializers[SerializationFormat(pending.SerializeFormat)]
	if !exists {
		return nil, fmt.Errorf("serializer not available for format: %d", pending.SerializeFormat)
	}

	message, err := serializer.Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("deserialization failed: %v", err)
	}

	return message, nil
}

// cleanupRoutine periodically cleans up timed-out pending messages
func (mr *MessageReassembler) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mr.mutex.Lock()
		now := time.Now().UnixNano()

		for messageID, pending := range mr.pendingMessages {
			if now-pending.FirstReceived > mr.timeout.Nanoseconds() {
				delete(mr.pendingMessages, messageID)
			}
		}

		mr.mutex.Unlock()
	}
}

// GetPendingCount returns the number of pending messages
func (mr *MessageReassembler) GetPendingCount() int {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()
	return len(mr.pendingMessages)
}

// GetPendingInfo returns information about pending messages
func (mr *MessageReassembler) GetPendingInfo() map[uint64]int {
	mr.mutex.RLock()
	defer mr.mutex.RUnlock()

	info := make(map[uint64]int)
	for messageID, pending := range mr.pendingMessages {
		info[messageID] = len(pending.ReceivedFragments)
	}
	return info
}