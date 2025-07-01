package serialization

import (
	"fmt"
	"math"
	"sync"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// Manager implements SerializationManager interface
type Manager struct {
	serializers map[SerializationFormat]MessageSerializer
	compressors map[CompressionType]MessageCompressor
	analyzer    DataAnalyzer
	defaultFormat SerializationFormat
	stats       *SerializationStats
	mutex       sync.RWMutex
}

// NewManager creates a new serialization manager
func NewManager() *Manager {
	sm := &Manager{
		serializers:   make(map[SerializationFormat]MessageSerializer),
		compressors:   make(map[CompressionType]MessageCompressor),
		analyzer:      NewDataAnalyzer(),
		defaultFormat: FormatBinary, // Use binary as default for space efficiency
		stats: &SerializationStats{
			FormatDistribution:      make(map[SerializationFormat]int64),
			CompressionDistribution: make(map[CompressionType]int64),
		},
	}

	// Register default serializers
	sm.RegisterSerializer(FormatBinary, NewBinarySerializer())
	sm.RegisterSerializer(FormatJSON, NewJSONSerializer())

	// Register default compressors
	sm.RegisterCompressor(CompressionLZ4, NewLZ4Compressor())
	sm.RegisterCompressor(CompressionGzip, NewGzipCompressor())

	return sm
}

// RegisterSerializer registers a message serializer
func (sm *Manager) RegisterSerializer(format SerializationFormat, serializer MessageSerializer) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.serializers[format] = serializer
}

// RegisterCompressor registers a message compressor
func (sm *Manager) RegisterCompressor(ctype CompressionType, compressor MessageCompressor) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.compressors[ctype] = compressor
}

// Serialize serializes a message with specific format
func (sm *Manager) Serialize(message *types.SGCSFMessage, format SerializationFormat) ([]byte, error) {
	sm.mutex.RLock()
	serializer, exists := sm.serializers[format]
	sm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("serializer not found for format: %d", format)
	}

	data, err := serializer.Serialize(message)
	if err != nil {
		sm.updateStats(err)
		return nil, err
	}

	sm.updateStatsSuccess(format, len(data))
	return data, nil
}

// Deserialize deserializes data with specific format
func (sm *Manager) Deserialize(data []byte, format SerializationFormat) (*types.SGCSFMessage, error) {
	sm.mutex.RLock()
	serializer, exists := sm.serializers[format]
	sm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("serializer not found for format: %d", format)
	}

	message, err := serializer.Deserialize(data)
	if err != nil {
		sm.updateStats(err)
		return nil, err
	}

	return message, nil
}

// SerializeAdaptive automatically chooses the best serialization format
func (sm *Manager) SerializeAdaptive(message *types.SGCSFMessage) ([]byte, SerializationFormat, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	bestFormat := sm.defaultFormat
	bestSize := math.MaxInt32
	var bestData []byte

	// Try different formats to find the most compact one
	formats := []SerializationFormat{FormatBinary, FormatJSON}
	
	for _, format := range formats {
		serializer, exists := sm.serializers[format]
		if !exists {
			continue
		}

		data, err := serializer.Serialize(message)
		if err != nil {
			continue
		}

		if len(data) < bestSize {
			bestSize = len(data)
			bestFormat = format
			bestData = data
		}
	}

	if bestData == nil {
		return nil, 0, fmt.Errorf("no suitable serializer found")
	}

	sm.updateStatsSuccess(bestFormat, len(bestData))
	return bestData, bestFormat, nil
}

// Compress compresses data with specific compression type
func (sm *Manager) Compress(data []byte, ctype CompressionType) ([]byte, error) {
	if ctype == CompressionNone {
		return data, nil
	}

	sm.mutex.RLock()
	compressor, exists := sm.compressors[ctype]
	sm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("compressor not found for type: %d", ctype)
	}

	compressed, err := compressor.Compress(data)
	if err != nil {
		return nil, err
	}

	sm.mutex.Lock()
	sm.stats.CompressionDistribution[ctype]++
	sm.mutex.Unlock()

	return compressed, nil
}

// Decompress decompresses data with specific compression type
func (sm *Manager) Decompress(data []byte, ctype CompressionType) ([]byte, error) {
	if ctype == CompressionNone {
		return data, nil
	}

	sm.mutex.RLock()
	compressor, exists := sm.compressors[ctype]
	sm.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("compressor not found for type: %d", ctype)
	}

	return compressor.Decompress(data)
}

// CompressAdaptive automatically chooses the best compression
func (sm *Manager) CompressAdaptive(data []byte, context CompressionContext) ([]byte, CompressionType, error) {
	// Don't compress small data
	if len(data) < 200 {
		return data, CompressionNone, nil
	}

	// Analyze data characteristics
	dataType := sm.analyzer.AnalyzeDataType(data)
	entropy := sm.analyzer.CalculateEntropy(data)

	// Choose compression type based on context and data characteristics
	var ctype CompressionType
	switch {
	case entropy < 0.3: // Low entropy data, high repetition
		ctype = CompressionGzip // Higher compression ratio
	case dataType == "json" || dataType == "text":
		ctype = CompressionGzip // Good for text data
	case context.LatencySensitive:
		ctype = CompressionLZ4 // Fast compression
	default:
		ctype = CompressionLZ4 // Balanced choice
	}

	// Try to compress
	compressed, err := sm.Compress(data, ctype)
	if err != nil {
		return data, CompressionNone, err
	}

	// Check compression effectiveness
	compressionRatio := float64(len(compressed)) / float64(len(data))
	if compressionRatio > 0.9 { // Not much compression gained
		return data, CompressionNone, nil
	}

	return compressed, ctype, nil
}

// GetSerializer returns a serializer for the given format
func (sm *Manager) GetSerializer(format SerializationFormat) (MessageSerializer, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	serializer, exists := sm.serializers[format]
	if !exists {
		return nil, fmt.Errorf("serializer not found for format: %d", format)
	}
	return serializer, nil
}

// GetCompressor returns a compressor for the given type
func (sm *Manager) GetCompressor(ctype CompressionType) (MessageCompressor, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	compressor, exists := sm.compressors[ctype]
	if !exists {
		return nil, fmt.Errorf("compressor not found for type: %d", ctype)
	}
	return compressor, nil
}

// EstimateSize estimates the serialized size of a message
func (sm *Manager) EstimateSize(message *types.SGCSFMessage, format SerializationFormat) int {
	// Base size estimation
	baseSize := 30 // Headers and metadata

	// Topic length
	baseSize += len(message.Topic)

	// Payload size
	baseSize += len(message.Payload)

	// Headers and metadata
	for k, v := range message.Headers {
		baseSize += len(k) + len(v) + 4 // Key + value + overhead
	}
	for k, v := range message.Metadata {
		baseSize += len(k) + len(v) + 4 // Key + value + overhead
	}

	// Format-specific adjustments
	switch format {
	case FormatJSON:
		return int(float64(baseSize) * 1.5) // JSON has more overhead
	case FormatBinary:
		return baseSize // Binary is most compact
	default:
		return int(float64(baseSize) * 1.2) // Conservative estimate
	}
}

// GetStats returns serialization statistics
func (sm *Manager) GetStats() *SerializationStats {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Create a copy of stats
	stats := &SerializationStats{
		TotalMessages:           sm.stats.TotalMessages,
		TotalBytes:              sm.stats.TotalBytes,
		CompressionRatio:        sm.stats.CompressionRatio,
		AverageMessageSize:      sm.stats.AverageMessageSize,
		ErrorCount:              sm.stats.ErrorCount,
		FormatDistribution:      make(map[SerializationFormat]int64),
		CompressionDistribution: make(map[CompressionType]int64),
	}

	for k, v := range sm.stats.FormatDistribution {
		stats.FormatDistribution[k] = v
	}
	for k, v := range sm.stats.CompressionDistribution {
		stats.CompressionDistribution[k] = v
	}

	return stats
}

// updateStatsSuccess updates statistics for successful operations
func (sm *Manager) updateStatsSuccess(format SerializationFormat, size int) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.stats.TotalMessages++
	sm.stats.TotalBytes += int64(size)
	sm.stats.FormatDistribution[format]++
	sm.stats.AverageMessageSize = float64(sm.stats.TotalBytes) / float64(sm.stats.TotalMessages)
}

// updateStats updates statistics for errors
func (sm *Manager) updateStats(err error) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.stats.ErrorCount++
}