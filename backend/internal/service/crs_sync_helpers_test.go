package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSelectedSet(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		wantNil  bool
		wantSize int
REDACTED{
		{
			name:    "nil input returns nil (backward compatible: create all)",
			ids:     nil,
			wantNil: true,
	REDACTED,
		{
			name:     "empty slice returns empty map (create none)",
			ids:      []string{REDACTED,
			wantNil:  false,
			wantSize: 0,
	REDACTED,
		{
			name:     "single ID",
			ids:      []string{"abc-123"REDACTED,
			wantNil:  false,
			wantSize: 1,
	REDACTED,
		{
			name:     "multiple IDs",
			ids:      []string{"a", "b", "c"REDACTED,
			wantNil:  false,
			wantSize: 3,
	REDACTED,
		{
			name:     "duplicate IDs are deduplicated",
			ids:      []string{"a", "a", "b"REDACTED,
			wantNil:  false,
			wantSize: 2,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectedSet(tt.ids)
			if tt.wantNil {
				if got != nil {
					t.Errorf("buildSelectedSet(%v) = %v, want nil", tt.ids, got)
			REDACTED
				return
		REDACTED
			if got == nil {
				t.Fatalf("buildSelectedSet(%v) = nil, want non-nil map", tt.ids)
		REDACTED
			if len(got) != tt.wantSize {
				t.Errorf("buildSelectedSet(%v) has %d entries, want %d", tt.ids, len(got), tt.wantSize)
		REDACTED
			// Verify all unique IDs are present
			for _, id := range tt.ids {
				if _, ok := got[id]; !ok {
					t.Errorf("buildSelectedSet(%v) missing key %q", tt.ids, id)
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestShouldCreateAccount(t *testing.T) {
	tests := []struct {
		name        string
		crsID       string
		selectedSet map[string]struct{REDACTED
		want        bool
REDACTED{
		{
			name:        "nil set allows all (backward compatible)",
			crsID:       "any-id",
			selectedSet: nil,
			want:        true,
	REDACTED,
		{
			name:        "empty set blocks all",
			crsID:       "any-id",
			selectedSet: map[string]struct{REDACTED{REDACTED,
			want:        false,
	REDACTED,
		{
			name:        "ID in set is allowed",
			crsID:       "abc-123",
			selectedSet: map[string]struct{REDACTED{"abc-123": {REDACTED, "def-456": {REDACTEDREDACTED,
			want:        true,
	REDACTED,
		{
			name:        "ID not in set is blocked",
			crsID:       "xyz-789",
			selectedSet: map[string]struct{REDACTED{"abc-123": {REDACTED, "def-456": {REDACTEDREDACTED,
			want:        false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateAccount(tt.crsID, tt.selectedSet)
			if got != tt.want {
				t.Errorf("shouldCreateAccount(%q, %v) = %v, want %v",
					tt.crsID, tt.selectedSet, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestReconcileCRSUpstreamBillingProbeExtra(t *testing.T) {
	remote := map[string]any{
		"crs_account_id":                       "remote-1",
		UpstreamBillingProbeEnabledExtraKey:    true,
		UpstreamBillingRateSyncEnabledExtraKey: true,
		UpstreamBillingProbeExtraKey:           map[string]any{"status": "remote"REDACTED,
REDACTED

	t.Run("create drops remote managed fields", func(t *testing.T) {
		extra := mergeMap(nil, remote)
		reconcileCRSUpstreamBillingProbeExtra(nil, PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "new"REDACTED, extra)
		require.NotContains(t, extra, UpstreamBillingProbeEnabledExtraKey)
		require.NotContains(t, extra, UpstreamBillingRateSyncEnabledExtraKey)
		require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
REDACTED)

	existing := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
REDACTED"api_key": "local", "base_url": "http://127.0.0.1:8080"REDACTED,
		Extra: map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    false,
			UpstreamBillingRateSyncEnabledExtraKey: false,
			UpstreamBillingProbeExtraKey:           map[string]any{"status": "local"REDACTED,
	REDACTED,
REDACTED

	t.Run("same identity keeps local state", func(t *testing.T) {
		extra := mergeMap(existing.Extra, remote)
		reconcileCRSUpstreamBillingProbeExtra(existing, existing.Platform, existing.Type, mergeMap(existing.Credentials, nil), extra)
		require.Equal(t, false, extra[UpstreamBillingProbeEnabledExtraKey])
		require.Equal(t, false, extra[UpstreamBillingRateSyncEnabledExtraKey])
		require.Equal(t, map[string]any{"status": "local"REDACTED, extra[UpstreamBillingProbeExtraKey])
REDACTED)

	t.Run("same identity preserves enabled rate sync", func(t *testing.T) {
		enabled := *existing
		enabled.Extra = mergeMap(existing.Extra, map[string]any{
			UpstreamBillingProbeEnabledExtraKey:    true,
			UpstreamBillingRateSyncEnabledExtraKey: true,
	REDACTED)
		extra := mergeMap(enabled.Extra, remote)
		reconcileCRSUpstreamBillingProbeExtra(&enabled, enabled.Platform, enabled.Type, mergeMap(enabled.Credentials, nil), extra)
		require.Equal(t, true, extra[UpstreamBillingProbeEnabledExtraKey])
		require.Equal(t, true, extra[UpstreamBillingRateSyncEnabledExtraKey])
REDACTED)

	t.Run("identity change keeps enabled and clears snapshot", func(t *testing.T) {
		extra := mergeMap(existing.Extra, remote)
		reconcileCRSUpstreamBillingProbeExtra(existing, PlatformOpenAI, AccountTypeAPIKey, map[string]any{"api_key": "changed"REDACTED, extra)
		require.Equal(t, false, extra[UpstreamBillingProbeEnabledExtraKey])
		require.Equal(t, false, extra[UpstreamBillingRateSyncEnabledExtraKey])
		require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
REDACTED)

	// API-key 平台间切换：探测资格保留（放宽后不再限 OpenAI），开关沿用本地值，
	// 但平台属于探测身份，快照必须作废。
	for _, target := range []struct {
		name     string
		platform string
REDACTED{
		{name: "anthropic api key", platform: PlatformAnthropicREDACTED,
		{name: "gemini api key", platform: PlatformGeminiREDACTED,
REDACTED {
		t.Run(target.name+" keeps enabled and clears snapshot", func(t *testing.T) {
			extra := mergeMap(existing.Extra, remote)
			reconcileCRSUpstreamBillingProbeExtra(existing, target.platform, AccountTypeAPIKey, existing.Credentials, extra)
			require.Equal(t, false, extra[UpstreamBillingProbeEnabledExtraKey])
			require.Equal(t, false, extra[UpstreamBillingRateSyncEnabledExtraKey])
			require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
	REDACTED)
REDACTED

	for _, target := range []struct {
		name     string
		platform string
		typeName string
REDACTED{
		{name: "anthropic oauth", platform: PlatformAnthropic, typeName: AccountTypeOAuthREDACTED,
		{name: "openai oauth", platform: PlatformOpenAI, typeName: AccountTypeOAuthREDACTED,
		{name: "gemini oauth", platform: PlatformGemini, typeName: AccountTypeOAuthREDACTED,
REDACTED {
		t.Run(target.name+" removes inapplicable state", func(t *testing.T) {
			extra := mergeMap(existing.Extra, remote)
			reconcileCRSUpstreamBillingProbeExtra(existing, target.platform, target.typeName, existing.Credentials, extra)
			require.NotContains(t, extra, UpstreamBillingProbeEnabledExtraKey)
			require.NotContains(t, extra, UpstreamBillingRateSyncEnabledExtraKey)
			require.NotContains(t, extra, UpstreamBillingProbeExtraKey)
	REDACTED)
REDACTED
REDACTED
