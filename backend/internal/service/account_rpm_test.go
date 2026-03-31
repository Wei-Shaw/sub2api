package service

import (
	"encoding/json"
	"testing"
)

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
		{"json.Number value", map[string]any{"base_rpm": json.Number("25")REDACTED, 25REDACTED,
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
		name        string
		concurrency int
		extra       map[string]any
		expected    int
REDACTED{
		// 基础退化
		{"nil extra", 0, nil, 0REDACTED,
		{"no keys", 0, map[string]any{REDACTED, 0REDACTED,
		{"base_rpm=0", 0, map[string]any{"base_rpm": 0REDACTED, 0REDACTED,

		// 新公式: concurrency + maxSessions, floor = base/5
		{"conc=3 sess=10 → 13", 3, map[string]any{"base_rpm": 15, "max_sessions": 10REDACTED, 13REDACTED,
		{"conc=2 sess=5 → 7", 2, map[string]any{"base_rpm": 10, "max_sessions": 5REDACTED, 7REDACTED,
		{"conc=3 sess=15 → 18", 3, map[string]any{"base_rpm": 30, "max_sessions": 15REDACTED, 18REDACTED,

		// floor 生效 (conc+sess < base/5)
		{"conc=0 sess=0 base=15 → floor 3", 0, map[string]any{"base_rpm": 15REDACTED, 3REDACTED,
		{"conc=0 sess=0 base=10 → floor 2", 0, map[string]any{"base_rpm": 10REDACTED, 2REDACTED,
		{"conc=0 sess=0 base=1 → floor 1", 0, map[string]any{"base_rpm": 1REDACTED, 1REDACTED,
		{"conc=0 sess=0 base=4 → floor 1", 0, map[string]any{"base_rpm": 4REDACTED, 1REDACTED,
		{"conc=1 sess=0 base=15 → floor 3", 1, map[string]any{"base_rpm": 15REDACTED, 3REDACTED,

		// 手动 override
		{"custom buffer=5", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 5, "max_sessions": 10REDACTED, 5REDACTED,
		{"custom buffer=0 fallback", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": 0, "max_sessions": 10REDACTED, 13REDACTED,
		{"custom buffer negative fallback", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": -1, "max_sessions": 10REDACTED, 13REDACTED,
		{"custom buffer with float", 3, map[string]any{"base_rpm": 10, "rpm_sticky_buffer": float64(7)REDACTED, 7REDACTED,

		// 负值 clamp
		{"negative concurrency clamped", -5, map[string]any{"base_rpm": 15, "max_sessions": 10REDACTED, 10REDACTED,
		{"negative maxSessions clamped", 3, map[string]any{"base_rpm": 15, "max_sessions": -5REDACTED, 3REDACTED,

		// 高并发低会话
		{"conc=10 sess=5 → 15", 10, map[string]any{"base_rpm": 10, "max_sessions": 5REDACTED, 15REDACTED,

		// json.Number
		{"json.Number base_rpm", 3, map[string]any{"base_rpm": json.Number("10"), "max_sessions": json.Number("5")REDACTED, 8REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Concurrency: tt.concurrency, Extra: tt.extraREDACTED
			if got := a.GetRPMStickyBuffer(); got != tt.expected {
				t.Errorf("GetRPMStickyBuffer() = %d, want %d", got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED
