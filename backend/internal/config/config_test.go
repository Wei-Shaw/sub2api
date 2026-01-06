package config

import (
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestNormalizeRunMode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
REDACTED{
		{"simple", "simple"REDACTED,
		{"SIMPLE", "simple"REDACTED,
		{"standard", "standard"REDACTED,
		{"invalid", "standard"REDACTED,
		{"", "standard"REDACTED,
REDACTED

	for _, tt := range tests {
		result := NormalizeRunMode(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeRunMode(%q) = %q, want %q", tt.input, result, tt.expected)
	REDACTED
REDACTED
REDACTED

func TestLoadDefaultSchedulingConfig(t *testing.T) {
	viper.Reset()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 3 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 3", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
REDACTED
	if cfg.Gateway.Scheduling.StickySessionWaitTimeout != 45*time.Second {
		t.Fatalf("StickySessionWaitTimeout = %v, want 45s", cfg.Gateway.Scheduling.StickySessionWaitTimeout)
REDACTED
	if cfg.Gateway.Scheduling.FallbackWaitTimeout != 30*time.Second {
		t.Fatalf("FallbackWaitTimeout = %v, want 30s", cfg.Gateway.Scheduling.FallbackWaitTimeout)
REDACTED
	if cfg.Gateway.Scheduling.FallbackMaxWaiting != 100 {
		t.Fatalf("FallbackMaxWaiting = %d, want 100", cfg.Gateway.Scheduling.FallbackMaxWaiting)
REDACTED
	if !cfg.Gateway.Scheduling.LoadBatchEnabled {
		t.Fatalf("LoadBatchEnabled = false, want true")
REDACTED
	if cfg.Gateway.Scheduling.SlotCleanupInterval != 30*time.Second {
		t.Fatalf("SlotCleanupInterval = %v, want 30s", cfg.Gateway.Scheduling.SlotCleanupInterval)
REDACTED
REDACTED

func TestLoadSchedulingConfigFromEnv(t *testing.T) {
	viper.Reset()
	t.Setenv("GATEWAY_SCHEDULING_STICKY_SESSION_MAX_WAITING", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Gateway.Scheduling.StickySessionMaxWaiting != 5 {
		t.Fatalf("StickySessionMaxWaiting = %d, want 5", cfg.Gateway.Scheduling.StickySessionMaxWaiting)
REDACTED
REDACTED

func TestLoadDefaultSecurityToggles(t *testing.T) {
	viper.Reset()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
REDACTED

	if cfg.Security.URLAllowlist.Enabled {
		t.Fatalf("URLAllowlist.Enabled = true, want false")
REDACTED
	if !cfg.Security.URLAllowlist.AllowInsecureHTTP {
		t.Fatalf("URLAllowlist.AllowInsecureHTTP = false, want true")
REDACTED
	if !cfg.Security.URLAllowlist.AllowPrivateHosts {
		t.Fatalf("URLAllowlist.AllowPrivateHosts = false, want true")
REDACTED
	if cfg.Security.ResponseHeaders.Enabled {
		t.Fatalf("ResponseHeaders.Enabled = true, want false")
REDACTED
REDACTED
