package serialization

import (
	"encoding/json"

	"github.com/sgcsf/sgcsf-go/internal/types"
)

// JSONSerializer implements MessageSerializer for JSON format
type JSONSerializer struct{}

// NewJSONSerializer creates a new JSON serializer
func NewJSONSerializer() *JSONSerializer {
	return &JSONSerializer{}
}

// Serialize implements MessageSerializer interface
func (js *JSONSerializer) Serialize(message *types.SGCSFMessage) ([]byte, error) {
	return json.Marshal(message)
}

// Deserialize implements MessageSerializer interface
func (js *JSONSerializer) Deserialize(data []byte) (*types.SGCSFMessage, error) {
	var message types.SGCSFMessage
	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// GetFormat returns the serialization format
func (js *JSONSerializer) GetFormat() SerializationFormat {
	return FormatJSON
}

// GetMaxPayloadSize returns the maximum payload size for JSON
func (js *JSONSerializer) GetMaxPayloadSize() int {
	return 600 // Conservative estimate due to JSON overhead
}