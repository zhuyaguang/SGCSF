package serialization

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"time"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// Magic numbers and constants for binary format
const (
	SGCSF_MAGIC   = 0x5347 // "SG"
	SGCSF_VERSION = 0x01
)

// MessageFlags defines flag bits for binary format
type MessageFlags uint8

const (
	FlagSync        MessageFlags = 1 << 0
	FlagCompressed  MessageFlags = 1 << 1
	FlagFragmented  MessageFlags = 1 << 2
	FlagHasHeaders  MessageFlags = 1 << 3
	FlagPriorityMask MessageFlags = 3 << 4 // 2 bits for priority
	FlagQoSMask     MessageFlags = 3 << 6  // 2 bits for QoS
)

// BinarySerializer implements MessageSerializer for custom binary format
type BinarySerializer struct {
	baseTimestamp int64 // Base timestamp for relative time encoding
}

// NewBinarySerializer creates a new binary serializer
func NewBinarySerializer() *BinarySerializer {
	return &BinarySerializer{
		baseTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(),
	}
}

// Serialize implements MessageSerializer interface
func (bs *BinarySerializer) Serialize(message *types.SGCSFMessage) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 800)) // Pre-allocate MTU size

	// 1. Write magic number and version
	if err := binary.Write(buf, binary.LittleEndian, uint16(SGCSF_MAGIC)); err != nil {
		return nil, err
	}
	if err := buf.WriteByte(SGCSF_VERSION); err != nil {
		return nil, err
	}

	// 2. Build and write flags
	flags := bs.buildFlags(message)
	if err := buf.WriteByte(uint8(flags)); err != nil {
		return nil, err
	}

	// 3. Write message ID (varint)
	bs.writeVarInt(buf, message.GetIDAsInt())

	// 4. Write relative timestamp (saves space)
	relativeTime := message.Timestamp - bs.baseTimestamp
	if relativeTime < 0 {
		relativeTime = 0
	}
	bs.writeVarInt(buf, uint64(relativeTime))

	// 5. Write topic (varstring)
	bs.writeVarString(buf, message.Topic)

	// 6. Write optional sync fields
	if flags&FlagSync != 0 {
		bs.writeVarInt(buf, message.GetRequestIDAsInt())
		if message.ResponseTo != "" {
			bs.writeVarInt(buf, message.GetResponseToAsInt())
		} else {
			bs.writeVarInt(buf, 0)
		}
		bs.writeVarInt(buf, uint64(message.Timeout))
	}

	// 7. Write stream fields if applicable
	if message.StreamID != "" {
		bs.writeVarString(buf, message.StreamID)
		buf.WriteByte(uint8(message.StreamType))
		
		var streamFlags uint8
		if message.IsStreamStart {
			streamFlags |= 0x01
		}
		if message.IsStreamEnd {
			streamFlags |= 0x02
		}
		buf.WriteByte(streamFlags)
	}

	// 8. Write optional headers and metadata
	if flags&FlagHasHeaders != 0 {
		bs.writeHeaders(buf, message.Headers, message.Metadata)
	}

	// 9. Write content type and encoding if present
	if message.ContentType != "" {
		bs.writeVarString(buf, message.ContentType)
	} else {
		bs.writeVarInt(buf, 0) // Empty string
	}
	
	if message.ContentEncoding != "" {
		bs.writeVarString(buf, message.ContentEncoding)
	} else {
		bs.writeVarInt(buf, 0) // Empty string
	}

	// 10. Write payload length and data
	bs.writeVarInt(buf, uint64(len(message.Payload)))
	if len(message.Payload) > 0 {
		buf.Write(message.Payload)
	}

	return buf.Bytes(), nil
}

// Deserialize implements MessageSerializer interface
func (bs *BinarySerializer) Deserialize(data []byte) (*types.SGCSFMessage, error) {
	buf := bytes.NewReader(data)

	// 1. Verify magic number and version
	var magic uint16
	if err := binary.Read(buf, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != SGCSF_MAGIC {
		return nil, fmt.Errorf("invalid magic number: %x", magic)
	}

	version, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	if version != SGCSF_VERSION {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}

	// 2. Read flags
	flagsByte, err := buf.ReadByte()
	if err != nil {
		return nil, err
	}
	flags := MessageFlags(flagsByte)

	// 3. Initialize message
	message := &types.SGCSFMessage{
		Headers:  make(map[string]string),
		Metadata: make(map[string]string),
	}

	// 4. Read basic fields
	messageIDInt, err := bs.readVarInt(buf)
	if err != nil {
		return nil, err
	}
	message.ID = fmt.Sprintf("msg_%d", messageIDInt)

	relativeTime, err := bs.readVarInt(buf)
	if err != nil {
		return nil, err
	}
	message.Timestamp = int64(relativeTime) + bs.baseTimestamp

	message.Topic, err = bs.readVarString(buf)
	if err != nil {
		return nil, err
	}

	// 5. Parse flags
	message.IsSync = (flags & FlagSync) != 0
	if message.IsSync {
		message.Type = types.MessageTypeSync
	} else {
		message.Type = types.MessageTypeAsync
	}
	
	message.Priority = types.Priority((flags & FlagPriorityMask) >> 4)
	message.QoS = types.QoSLevel((flags & FlagQoSMask) >> 6)

	// 6. Read sync fields if present
	if message.IsSync {
		requestIDInt, err := bs.readVarInt(buf)
		if err != nil {
			return nil, err
		}
		if requestIDInt != 0 {
			message.RequestID = fmt.Sprintf("req_%d", requestIDInt)
		}

		responseToInt, err := bs.readVarInt(buf)
		if err != nil {
			return nil, err
		}
		if responseToInt != 0 {
			message.ResponseTo = fmt.Sprintf("msg_%d", responseToInt)
		}

		timeout, err := bs.readVarInt(buf)
		if err != nil {
			return nil, err
		}
		message.Timeout = int64(timeout)
	}

	// 7. Read stream fields if present
	if buf.Len() > 0 {
		// Check if we have stream ID (simplified check)
		streamIDLen, _ := bs.peekVarInt(buf)
		if streamIDLen > 0 && streamIDLen < 100 { // Reasonable stream ID length
			message.StreamID, err = bs.readVarString(buf)
			if err == nil && message.StreamID != "" {
				streamTypeByte, err := buf.ReadByte()
				if err != nil {
					return nil, err
				}
				message.StreamType = types.StreamType(streamTypeByte)

				streamFlags, err := buf.ReadByte()
				if err != nil {
					return nil, err
				}
				message.IsStreamStart = (streamFlags & 0x01) != 0
				message.IsStreamEnd = (streamFlags & 0x02) != 0
			}
		}
	}

	// 8. Read headers if present
	if (flags & FlagHasHeaders) != 0 {
		err := bs.readHeaders(buf, message)
		if err != nil {
			return nil, err
		}
	}

	// 9. Read content type and encoding
	message.ContentType, err = bs.readVarString(buf)
	if err != nil {
		return nil, err
	}

	message.ContentEncoding, err = bs.readVarString(buf)
	if err != nil {
		return nil, err
	}

	// 10. Read payload
	payloadLen, err := bs.readVarInt(buf)
	if err != nil {
		return nil, err
	}

	if payloadLen > 0 {
		message.Payload = make([]byte, payloadLen)
		_, err = io.ReadFull(buf, message.Payload)
		if err != nil {
			return nil, err
		}
	}

	return message, nil
}

// GetFormat returns the serialization format
func (bs *BinarySerializer) GetFormat() SerializationFormat {
	return FormatBinary
}

// GetMaxPayloadSize returns the maximum payload size
func (bs *BinarySerializer) GetMaxPayloadSize() int {
	return 750 // MTU 800 - ~50 bytes for headers
}

// writeVarInt writes a variable-length integer (similar to Protobuf varint)
func (bs *BinarySerializer) writeVarInt(buf *bytes.Buffer, value uint64) {
	for value >= 0x80 {
		buf.WriteByte(byte(value&0x7F | 0x80))
		value >>= 7
	}
	buf.WriteByte(byte(value & 0x7F))
}

// readVarInt reads a variable-length integer
func (bs *BinarySerializer) readVarInt(buf *bytes.Reader) (uint64, error) {
	var result uint64
	var shift uint
	for {
		b, err := buf.ReadByte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("varint overflow")
		}
	}
	return result, nil
}

// peekVarInt reads a varint without advancing the buffer
func (bs *BinarySerializer) peekVarInt(buf *bytes.Reader) (uint64, error) {
	// Save current position
	currentPos, _ := buf.Seek(0, io.SeekCurrent)
	
	// Read varint
	result, err := bs.readVarInt(buf)
	
	// Restore position
	buf.Seek(currentPos, io.SeekStart)
	
	return result, err
}

// writeVarString writes a variable-length string
func (bs *BinarySerializer) writeVarString(buf *bytes.Buffer, str string) {
	bs.writeVarInt(buf, uint64(len(str)))
	buf.WriteString(str)
}

// readVarString reads a variable-length string
func (bs *BinarySerializer) readVarString(buf *bytes.Reader) (string, error) {
	length, err := bs.readVarInt(buf)
	if err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	
	str := make([]byte, length)
	_, err = io.ReadFull(buf, str)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

// buildFlags constructs the flags byte from message properties
func (bs *BinarySerializer) buildFlags(message *types.SGCSFMessage) MessageFlags {
	var flags MessageFlags

	if message.IsSync || message.Type == types.MessageTypeSync {
		flags |= FlagSync
	}

	// Encode priority (0-3)
	priority := uint8(message.Priority) & 0x03
	flags |= MessageFlags(priority << 4)

	// Encode QoS (0-3)
	qos := uint8(message.QoS) & 0x03
	flags |= MessageFlags(qos << 6)

	// Check for extended headers
	if len(message.Headers) > 0 || len(message.Metadata) > 0 {
		flags |= FlagHasHeaders
	}

	return flags
}

// writeHeaders writes headers and metadata
func (bs *BinarySerializer) writeHeaders(buf *bytes.Buffer, headers, metadata map[string]string) {
	// Write headers count
	bs.writeVarInt(buf, uint64(len(headers)))
	for key, value := range headers {
		bs.writeVarString(buf, key)
		bs.writeVarString(buf, value)
	}

	// Write metadata count
	bs.writeVarInt(buf, uint64(len(metadata)))
	for key, value := range metadata {
		bs.writeVarString(buf, key)
		bs.writeVarString(buf, value)
	}
}

// readHeaders reads headers and metadata
func (bs *BinarySerializer) readHeaders(buf *bytes.Reader, message *types.SGCSFMessage) error {
	// Read headers
	headerCount, err := bs.readVarInt(buf)
	if err != nil {
		return err
	}

	for i := uint64(0); i < headerCount; i++ {
		key, err := bs.readVarString(buf)
		if err != nil {
			return err
		}
		value, err := bs.readVarString(buf)
		if err != nil {
			return err
		}
		message.Headers[key] = value
	}

	// Read metadata
	metadataCount, err := bs.readVarInt(buf)
	if err != nil {
		return err
	}

	for i := uint64(0); i < metadataCount; i++ {
		key, err := bs.readVarString(buf)
		if err != nil {
			return err
		}
		value, err := bs.readVarString(buf)
		if err != nil {
			return err
		}
		message.Metadata[key] = value
	}

	return nil
}

// CRC32Checksummer implements Checksummer interface
type CRC32Checksummer struct{}

// NewCRC32Checksummer creates a new CRC32 checksummer
func NewCRC32Checksummer() *CRC32Checksummer {
	return &CRC32Checksummer{}
}

// Calculate computes CRC32 checksum
func (c *CRC32Checksummer) Calculate(data []byte) string {
	checksum := crc32.ChecksumIEEE(data)
	return fmt.Sprintf("%08x", checksum)
}

// Verify verifies CRC32 checksum
func (c *CRC32Checksummer) Verify(data []byte, checksum string) bool {
	calculated := c.Calculate(data)
	return calculated == checksum
}