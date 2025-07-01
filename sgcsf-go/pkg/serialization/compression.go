package serialization

import (
	"bytes"
	"compress/gzip"
	"io"
	"math"
)

// LZ4Compressor implements MessageCompressor for LZ4 compression
type LZ4Compressor struct{}

// NewLZ4Compressor creates a new LZ4 compressor
func NewLZ4Compressor() *LZ4Compressor {
	return &LZ4Compressor{}
}

// Compress compresses data using LZ4 (simplified implementation)
func (lz4 *LZ4Compressor) Compress(data []byte) ([]byte, error) {
	// For this demo, we'll use a simple run-length encoding
	// In production, you would use actual LZ4 library
	return lz4.simpleCompress(data), nil
}

// Decompress decompresses LZ4 data
func (lz4 *LZ4Compressor) Decompress(data []byte) ([]byte, error) {
	return lz4.simpleDecompress(data), nil
}

// GetType returns the compression type
func (lz4 *LZ4Compressor) GetType() CompressionType {
	return CompressionLZ4
}

// simpleCompress implements a basic compression algorithm
func (lz4 *LZ4Compressor) simpleCompress(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	var compressed bytes.Buffer
	i := 0
	for i < len(data) {
		current := data[i]
		count := 1

		// Count consecutive identical bytes
		for i+count < len(data) && data[i+count] == current && count < 255 {
			count++
		}

		if count > 3 {
			// Use compression for runs of 4 or more
			compressed.WriteByte(0xFF) // Escape byte
			compressed.WriteByte(byte(count))
			compressed.WriteByte(current)
		} else {
			// Write bytes directly
			for j := 0; j < count; j++ {
				if current == 0xFF {
					compressed.WriteByte(0xFF)
					compressed.WriteByte(0x00) // Escaped 0xFF
				} else {
					compressed.WriteByte(current)
				}
			}
		}
		i += count
	}

	return compressed.Bytes()
}

// simpleDecompress decompresses the simple compression format
func (lz4 *LZ4Compressor) simpleDecompress(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	var decompressed bytes.Buffer
	i := 0
	for i < len(data) {
		if data[i] == 0xFF && i+1 < len(data) {
			if data[i+1] == 0x00 {
				// Escaped 0xFF
				decompressed.WriteByte(0xFF)
				i += 2
			} else if i+2 < len(data) {
				// Compressed run
				count := int(data[i+1])
				value := data[i+2]
				for j := 0; j < count; j++ {
					decompressed.WriteByte(value)
				}
				i += 3
			} else {
				decompressed.WriteByte(data[i])
				i++
			}
		} else {
			decompressed.WriteByte(data[i])
			i++
		}
	}

	return decompressed.Bytes()
}

// GzipCompressor implements MessageCompressor for Gzip compression
type GzipCompressor struct{}

// NewGzipCompressor creates a new Gzip compressor
func NewGzipCompressor() *GzipCompressor {
	return &GzipCompressor{}
}

// Compress compresses data using Gzip
func (gz *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	
	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}
	
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

// Decompress decompresses Gzip data
func (gz *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// GetType returns the compression type
func (gz *GzipCompressor) GetType() CompressionType {
	return CompressionGzip
}

// SimpleDataAnalyzer implements DataAnalyzer interface
type SimpleDataAnalyzer struct{}

// NewDataAnalyzer creates a new data analyzer
func NewDataAnalyzer() *SimpleDataAnalyzer {
	return &SimpleDataAnalyzer{}
}

// AnalyzeDataType analyzes the type of data
func (da *SimpleDataAnalyzer) AnalyzeDataType(data []byte) string {
	if len(data) == 0 {
		return "empty"
	}

	// Check for JSON
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return "json"
	}

	// Check for text (high ratio of printable characters)
	printableCount := 0
	for _, b := range data {
		if b >= 32 && b <= 126 {
			printableCount++
		}
	}

	if float64(printableCount)/float64(len(data)) > 0.7 {
		return "text"
	}

	return "binary"
}

// CalculateEntropy calculates the entropy of data
func (da *SimpleDataAnalyzer) CalculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// Count frequency of each byte
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// Calculate entropy
	entropy := 0.0
	length := float64(len(data))
	
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * math.Log2(p)
		}
	}

	return entropy / 8.0 // Normalize to 0-1 range
}

// EstimateCompressionRatio estimates compression ratio for given data and type
func (da *SimpleDataAnalyzer) EstimateCompressionRatio(data []byte, ctype CompressionType) float64 {
	entropy := da.CalculateEntropy(data)
	dataType := da.AnalyzeDataType(data)

	switch ctype {
	case CompressionLZ4:
		// LZ4 is fast but lower compression ratio
		if dataType == "text" || dataType == "json" {
			return 0.6 + entropy*0.3 // 60-90% of original size
		}
		return 0.8 + entropy*0.15 // 80-95% for binary

	case CompressionGzip:
		// Gzip has better compression ratio
		if dataType == "text" || dataType == "json" {
			return 0.3 + entropy*0.4 // 30-70% of original size
		}
		return 0.6 + entropy*0.3 // 60-90% for binary

	case CompressionSnappy:
		// Snappy is balanced
		if dataType == "text" || dataType == "json" {
			return 0.5 + entropy*0.3 // 50-80% of original size
		}
		return 0.7 + entropy*0.2 // 70-90% for binary

	default:
		return 1.0 // No compression
	}
}