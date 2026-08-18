package modelcatalog

// TokenRates is the runtime per-token form of a catalog price card.
type TokenRates struct {
	Input                         float64
	Output                        float64
	InputPriority                 float64
	OutputPriority                float64
	CacheWrite                    float64
	CacheWritePriority            float64
	CacheRead                     float64
	CacheReadPriority             float64
	ImageInput                    float64
	ImageOutput                   float64
	LongContextInputThreshold     int
	LongContextInputMultiplier    float64
	LongContextOutputMultiplier   float64
	LongContextThresholdInclusive bool
}

// Rates returns the per-token rates for a catalog entry.
func (e *Entry) Rates() TokenRates {
	if e == nil || e.Price == nil {
		return TokenRates{}
	}
	p := e.Price
	return TokenRates{
		Input:                         DerefPerToken(p.InputPerMTok),
		Output:                        DerefPerToken(p.OutputPerMTok),
		InputPriority:                 DerefPerToken(p.InputPriorityPerMTok),
		OutputPriority:                DerefPerToken(p.OutputPriorityPerMTok),
		CacheWrite:                    DerefPerToken(p.CacheWritePerMTok),
		CacheWritePriority:            DerefPerToken(p.CacheWritePriorityPerMTok),
		CacheRead:                     DerefPerToken(p.CacheReadPerMTok),
		CacheReadPriority:             DerefPerToken(p.CacheReadPriorityPerMTok),
		ImageInput:                    DerefPerToken(p.ImageInputPerMTok),
		ImageOutput:                   DerefPerToken(p.ImageOutputPerMTok),
		LongContextInputThreshold:     p.LongContextInputThreshold,
		LongContextInputMultiplier:    p.LongContextInputMultiplier,
		LongContextOutputMultiplier:   p.LongContextOutputMultiplier,
		LongContextThresholdInclusive: p.LongContextThresholdInclusive,
	}
}
