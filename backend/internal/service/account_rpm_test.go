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
