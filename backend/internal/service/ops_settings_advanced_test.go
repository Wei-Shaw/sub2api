package service

import (
	"context"
	"testing"
)

func TestGetOpsAdvancedSettings_DefaultHidesOpenAITokenStats(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repoREDACTED

	cfg, err := svc.GetOpsAdvancedSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOpsAdvancedSettings() error = %v", err)
REDACTED
	if cfg.DisplayOpenAITokenStats {
		t.Fatalf("DisplayOpenAITokenStats = true, want false by default")
REDACTED
	if repo.setCalls != 1 {
		t.Fatalf("expected defaults to be persisted once, got %d", repo.setCalls)
REDACTED
REDACTED

func TestUpdateOpsAdvancedSettings_PersistsOpenAITokenStatsVisibility(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repoREDACTED

	cfg := defaultOpsAdvancedSettings()
	cfg.DisplayOpenAITokenStats = true

	updated, err := svc.UpdateOpsAdvancedSettings(context.Background(), cfg)
	if err != nil {
		t.Fatalf("UpdateOpsAdvancedSettings() error = %v", err)
REDACTED
	if !updated.DisplayOpenAITokenStats {
		t.Fatalf("DisplayOpenAITokenStats = false, want true")
REDACTED

	reloaded, err := svc.GetOpsAdvancedSettings(context.Background())
	if err != nil {
		t.Fatalf("GetOpsAdvancedSettings() after update error = %v", err)
REDACTED
	if !reloaded.DisplayOpenAITokenStats {
		t.Fatalf("reloaded DisplayOpenAITokenStats = false, want true")
REDACTED
REDACTED
