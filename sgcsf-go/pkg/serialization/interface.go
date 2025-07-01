package serialization

import (
	"github.com/sgcsf/sgcsf-go/internal/types"
)

// SerializationFormat represents different serialization formats
type SerializationFormat uint8

const (
	FormatProtobuf    SerializationFormat = 1 // Protocol Buffers
	FormatMessagePack SerializationFormat = 2 // MessagePack
	FormatJSON        SerializationFormat = 3 // JSON (debugging)
	FormatBinary      SerializationFormat = 4 // Custom binary
	FormatAvro        SerializationFormat = 5 // Apache Avro
)

// MessageSerializer defines the interface for message serialization
type MessageSerializer interface {
	Serialize(message *types.SGCSFMessage) ([]byte, error)
	Deserialize(data []byte) (*types.SGCSFMessage, error)
	GetFormat() SerializationFormat
	GetMaxPayloadSize() int // Maximum payload size considering serialization overhead
}

// MessageCompressor defines the interface for message compression
type MessageCompressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
	GetType() CompressionType
}

// CompressionType represents different compression algorithms
type CompressionType uint8

const (
	CompressionNone   CompressionType = iota
	CompressionLZ4                    // Fast compression
	CompressionGzip                   // General purpose
	CompressionSnappy                 // Google Snappy
	CompressionBrotli                 // High compression ratio
)

// CompressionContext provides context for compression decisions
type CompressionContext struct {
	Priority         types.Priority
	LatencySensitive bool
	BandwidthLimited bool
	CPUConstrained   bool
}

// SerializationManager manages multiple serializers and compression
type SerializationManager interface {
	RegisterSerializer(format SerializationFormat, serializer MessageSerializer)
	RegisterCompressor(ctype CompressionType, compressor MessageCompressor)
	
	// Serialize with specific format
	Serialize(message *types.SGCSFMessage, format SerializationFormat) ([]byte, error)
	Deserialize(data []byte, format SerializationFormat) (*types.SGCSFMessage, error)
	
	// Adaptive serialization - automatically choose best format
	SerializeAdaptive(message *types.SGCSFMessage) ([]byte, SerializationFormat, error)
	
	// Compression
	Compress(data []byte, ctype CompressionType) ([]byte, error)
	Decompress(data []byte, ctype CompressionType) ([]byte, error)
	CompressAdaptive(data []byte, context CompressionContext) ([]byte, CompressionType, error)
	
	// Utilities
	GetSerializer(format SerializationFormat) (MessageSerializer, error)
	GetCompressor(ctype CompressionType) (MessageCompressor, error)
	EstimateSize(message *types.SGCSFMessage, format SerializationFormat) int
}

// DataAnalyzer analyzes data for optimal compression
type DataAnalyzer interface {
	AnalyzeDataType(data []byte) string
	CalculateEntropy(data []byte) float64
	EstimateCompressionRatio(data []byte, ctype CompressionType) float64
}

// Checksummer provides checksum calculation
type Checksummer interface {
	Calculate(data []byte) string
	Verify(data []byte, checksum string) bool
}

// SerializationStats provides statistics about serialization
type SerializationStats struct {
	TotalMessages       int64
	TotalBytes          int64
	CompressionRatio    float64
	AverageMessageSize  float64
	FormatDistribution  map[SerializationFormat]int64
	CompressionDistribution map[CompressionType]int64
	ErrorCount          int64
}