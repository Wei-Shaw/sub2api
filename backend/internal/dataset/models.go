package dataset

import (
	"encoding/json"
	"time"
)

// DatasetEntry represents a single LLM request/response pair for training dataset.
type DatasetEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Endpoint  string                 `json:"endpoint"` // "chat.completions", "responses"
	Request   map[string]interface{} `json:"request"`
	Response  map[string]interface{} `json:"response,omitempty"`
}

// ToJSON serializes the entry to JSON bytes (one line for JSONL).
func (e *DatasetEntry) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// SizeBytes returns the approximate size of this entry in bytes.
func (e *DatasetEntry) SizeBytes() int {
	data, err := e.ToJSON()
	if err != nil {
		return 0
	}
	return len(data)
}
