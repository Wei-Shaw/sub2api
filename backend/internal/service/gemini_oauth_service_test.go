package service

import "testing"

func TestInferGoogleOneTier(t *testing.T) {
	tests := []struct {
		name         string
		storageBytes int64
		expectedTier string
REDACTED{
		{"Negative storage", -1, TierGoogleOneUnknownREDACTED,
		{"Zero storage", 0, TierGoogleOneUnknownREDACTED,

		// Free tier boundary (15GB)
		{"Below free tier", 10 * GB, TierGoogleOneUnknownREDACTED,
		{"Just below free tier", StorageTierFree - 1, TierGoogleOneUnknownREDACTED,
		{"Free tier (15GB)", StorageTierFree, TierFreeREDACTED,

		// Basic tier boundary (100GB)
		{"Between free and basic", 50 * GB, TierFreeREDACTED,
		{"Just below basic tier", StorageTierBasic - 1, TierFreeREDACTED,
		{"Basic tier (100GB)", StorageTierBasic, TierGoogleOneBasicREDACTED,

		// Standard tier boundary (200GB)
		{"Between basic and standard", 150 * GB, TierGoogleOneBasicREDACTED,
		{"Just below standard tier", StorageTierStandard - 1, TierGoogleOneBasicREDACTED,
		{"Standard tier (200GB)", StorageTierStandard, TierGoogleOneStandardREDACTED,

		// AI Premium tier boundary (2TB)
		{"Between standard and premium", 1 * TB, TierGoogleOneStandardREDACTED,
		{"Just below AI Premium tier", StorageTierAIPremium - 1, TierGoogleOneStandardREDACTED,
		{"AI Premium tier (2TB)", StorageTierAIPremium, TierAIPremiumREDACTED,

		// Unlimited tier boundary (> 100TB)
		{"Between premium and unlimited", 50 * TB, TierAIPremiumREDACTED,
		{"At unlimited threshold (100TB)", StorageTierUnlimited, TierAIPremiumREDACTED,
		{"Unlimited tier (100TB+)", StorageTierUnlimited + 1, TierGoogleOneUnlimitedREDACTED,
		{"Unlimited tier (101TB+)", 101 * TB, TierGoogleOneUnlimitedREDACTED,
		{"Very large storage", 1000 * TB, TierGoogleOneUnlimitedREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferGoogleOneTier(tt.storageBytes)
			if result != tt.expectedTier {
				t.Errorf("inferGoogleOneTier(%d) = %s, want %s",
					tt.storageBytes, result, tt.expectedTier)
		REDACTED
	REDACTED)
REDACTED
REDACTED

