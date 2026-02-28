package service

import "testing"

func TestGetBaseRPM(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected int
REDACTED{
		{"nil extra", nil, 0REDACTED,
		{"no key", map[string]any{REDACTED, 0REDACTED,
		{"zero", map[string]any{"base_rpm": 0REDACTED, 0REDACTED,
		{"int value", map[string]any{"base_rpm": 15REDACTED, 15REDACTED,
		{"float value", map[string]any{"base_rpm": 15.0REDACTED, 15REDACTED,
		{"string value", map[string]any{"base_rpm": "15"REDACTED, 15REDACTED,
		{"negative value", map[string]any{"base_rpm": -5REDACTED, 0REDACTED,
		{"int64 value", map[string]any{"base_rpm": int64(20)REDACTED, 20REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extraREDACTED
			if got := a.GetBaseRPM(); got != tt.expected {
				t.Errorf("GetBaseRPM() = %d, want %d", got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetRPMStrategy(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected string
REDACTED{
		{"nil extra", nil, "tiered"REDACTED,
		{"no key", map[string]any{REDACTED, "tiered"REDACTED,
		{"tiered", map[string]any{"rpm_strategy": "tiered"REDACTED, "tiered"REDACTED,
		{"sticky_exempt", map[string]any{"rpm_strategy": "sticky_exempt"REDACTED, "sticky_exempt"REDACTED,
		{"invalid", map[string]any{"rpm_strategy": "foobar"REDACTED, "tiered"REDACTED,
		{"empty string fallback", map[string]any{"rpm_strategy": ""REDACTED, "tiered"REDACTED,
		{"numeric value fallback", map[string]any{"rpm_strategy": 123REDACTED, "tiered"REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extraREDACTED
			if got := a.GetRPMStrategy(); got != tt.expected {
				t.Errorf("GetRPMStrategy() = %q, want %q", got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCheckRPMSchedulability(t *testing.T) {
	tests := []struct {
		name       string
		extra      map[string]any
		currentRPM int
		expected   WindowCostSchedulability
REDACTED{
		{"disabled", map[string]any{REDACTED, 100, WindowCostSchedulableREDACTED,
		{"green zone", map[string]any{"base_rpm": 15REDACTED, 10, WindowCostSchedulableREDACTED,
		{"yellow zone tiered", map[string]any{"base_rpm": 15REDACTED, 15, WindowCostStickyOnlyREDACTED,
		{"red zone tiered", map[string]any{"base_rpm": 15REDACTED, 18, WindowCostNotSchedulableREDACTED,
		{"sticky_exempt at limit", map[string]any{"base_rpm": 15, "rpm_strategy": "sticky_exempt"REDACTED, 15, WindowCostStickyOnlyREDACTED,
		{"sticky_exempt over limit", map[string]any{"base_rpm": 15, "rpm_strategy": "sticky_exempt"REDACTED, 100, WindowCostStickyOnlyREDACTED,
		{"custom buffer", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5REDACTED, 14, WindowCostStickyOnlyREDACTED,
		{"custom buffer red", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5REDACTED, 15, WindowCostNotSchedulableREDACTED,
		{"base_rpm=1 green", map[string]any{"base_rpm": 1REDACTED, 0, WindowCostSchedulableREDACTED,
		{"base_rpm=1 yellow (at limit)", map[string]any{"base_rpm": 1REDACTED, 1, WindowCostStickyOnlyREDACTED,
		{"base_rpm=1 red (at limit+buffer)", map[string]any{"base_rpm": 1REDACTED, 2, WindowCostNotSchedulableREDACTED,
		{"negative currentRPM", map[string]any{"base_rpm": 15REDACTED, -1, WindowCostSchedulableREDACTED,
		{"base_rpm negative disabled", map[string]any{"base_rpm": -5REDACTED, 10, WindowCostSchedulableREDACTED,
		{"very high currentRPM", map[string]any{"base_rpm": 10REDACTED, 9999, WindowCostNotSchedulableREDACTED,
		{"sticky_exempt very high currentRPM", map[string]any{"base_rpm": 10, "rpm_strategy": "sticky_exempt"REDACTED, 9999, WindowCostStickyOnlyREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extraREDACTED
			if got := a.CheckRPMSchedulability(tt.currentRPM); got != tt.expected {
				t.Errorf("CheckRPMSchedulability(%d) = %d, want %d", tt.currentRPM, got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetRPMStickyBuffer(t *testing.T) {
	tests := []struct {
		name     string
		extra    map[string]any
		expected int
REDACTED{
		{"nil extra", nil, 0REDACTED,
		{"no keys", map[string]any{REDACTED, 0REDACTED,
		{"base_rpm=0", map[string]any{"base_rpm": 0REDACTED, 0REDACTED,
		{"base_rpm=1 min buffer 1", map[string]any{"base_rpm": 1REDACTED, 1REDACTED,
		{"base_rpm=4 min buffer 1", map[string]any{"base_rpm": 4REDACTED, 1REDACTED,
		{"base_rpm=5 buffer 1", map[string]any{"base_rpm": 5REDACTED, 1REDACTED,
		{"base_rpm=10 buffer 2", map[string]any{"base_rpm": 10REDACTED, 2REDACTED,
		{"base_rpm=15 buffer 3", map[string]any{"base_rpm": 15REDACTED, 3REDACTED,
		{"base_rpm=100 buffer 20", map[string]any{"base_rpm": 100REDACTED, 20REDACTED,
		{"custom buffer=5", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5REDACTED, 5REDACTED,
		{"custom buffer=0 fallback to default", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 0REDACTED, 2REDACTED,
		{"custom buffer negative fallback", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": -1REDACTED, 2REDACTED,
		{"custom buffer with float", map[string]any{"base_rpm": 10, "rpm_sticky_buffer": float64(7)REDACTED, 7REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Extra: tt.extraREDACTED
			if got := a.GetRPMStickyBuffer(); got != tt.expected {
				t.Errorf("GetRPMStickyBuffer() = %d, want %d", got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED
