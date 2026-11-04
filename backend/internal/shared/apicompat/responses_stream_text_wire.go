package apicompat

// Typed shapes avoid a map, key sorting and boxed values for every delta.
// Fields remain in the previous map encoder's lexical order; only optional
// identity fields use omitempty. Empty text and zero indexes must stay present.
type responsesTextDeltaWire struct {
	ContentIndex   int    `json:"content_index"`
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	Type           string `json:"type"`
}

type responsesTextDoneWire struct {
	ContentIndex   int    `json:"content_index"`
	ItemID         string `json:"item_id,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	Text           string `json:"text"`
	Type           string `json:"type"`
}

type responsesSummaryDeltaWire struct {
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	SummaryIndex   int    `json:"summary_index"`
	Type           string `json:"type"`
}

type responsesSummaryDoneWire struct {
	ItemID         string `json:"item_id,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	SummaryIndex   int    `json:"summary_index"`
	Text           string `json:"text"`
	Type           string `json:"type"`
}

type responsesToolDeltaWire struct {
	CallID         string `json:"call_id,omitempty"`
	Delta          string `json:"delta"`
	ItemID         string `json:"item_id,omitempty"`
	Name           string `json:"name,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	Type           string `json:"type"`
}

type responsesToolArgumentsDoneWire struct {
	Arguments      string `json:"arguments"`
	CallID         string `json:"call_id,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	Name           string `json:"name,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	Type           string `json:"type"`
}

type responsesToolInputDoneWire struct {
	CallID         string `json:"call_id,omitempty"`
	Input          string `json:"input"`
	ItemID         string `json:"item_id,omitempty"`
	Name           string `json:"name,omitempty"`
	OutputIndex    int    `json:"output_index"`
	SequenceNumber int    `json:"sequence_number"`
	Type           string `json:"type"`
}
