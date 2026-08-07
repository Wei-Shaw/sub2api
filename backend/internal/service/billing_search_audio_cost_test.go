package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateSearchCost(t *testing.T) {
	t.Parallel()
	s := &BillingService{REDACTED
	require.Equal(t, 0.0, s.CalculateSearchCost(0, floatPtr(10), 1).ActualCost)
	require.Equal(t, 0.0, s.CalculateSearchCost(5, nil, 1).ActualCost)
	price := 10.0
	cost := s.CalculateSearchCost(100, &price, 1.5)
	// 10 / 1000 * 100 * 1.5 = 1.5
	require.InDelta(t, 1.0, cost.TotalCost, 1e-9)
	require.InDelta(t, 1.5, cost.ActualCost, 1e-9)
REDACTED

func TestCalculateAudioCost(t *testing.T) {
	t.Parallel()
	s := &BillingService{REDACTED
	rt, tts, stt := 0.10, 15.0, 0.50
	cfg := &audioPriceConfig{RealtimePerMin: &rt, TTSPerMChars: &tts, STTPerHour: &sttREDACTED
	require.InDelta(t, 0.20, s.CalculateAudioCost("realtime", 2, cfg, 1).ActualCost, 1e-9)
	require.InDelta(t, 1.5, s.CalculateAudioCost("tts", 0.1, cfg, 1).ActualCost, 1e-9)
	require.InDelta(t, 0.25, s.CalculateAudioCost("stt", 0.5, cfg, 1).ActualCost, 1e-9)
	require.Equal(t, 0.0, s.CalculateAudioCost("unknown", 1, cfg, 1).ActualCost)
REDACTED

func floatPtr(v float64) *float64 { return &v REDACTED
