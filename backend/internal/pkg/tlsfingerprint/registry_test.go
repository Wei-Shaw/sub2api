package tlsfingerprint

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	// Should have exactly one profile (the default)
	if r.ProfileCount() != 1 {
		t.Errorf("expected 1 profile, got %d", r.ProfileCount())
REDACTED

	// Should have the default profile
	profile := r.GetDefaultProfile()
	if profile == nil {
		t.Error("expected default profile to exist")
REDACTED

	// Default profile name should be in the list
	names := r.ProfileNames()
	if len(names) != 1 || names[0] != DefaultProfileName {
		t.Errorf("expected profile names to be [%s], got %v", DefaultProfileName, names)
REDACTED
REDACTED

func TestRegisterProfile(t *testing.T) {
	r := NewRegistry()

	// Register a new profile
	customProfile := &Profile{
		Name:         "Custom Profile",
		EnableGREASE: true,
REDACTED
	r.RegisterProfile("custom", customProfile)

	// Should now have 2 profiles
	if r.ProfileCount() != 2 {
		t.Errorf("expected 2 profiles, got %d", r.ProfileCount())
REDACTED

	// Should be able to retrieve the custom profile
	retrieved := r.GetProfile("custom")
	if retrieved == nil {
		t.Fatal("expected custom profile to exist")
REDACTED
	if retrieved.Name != "Custom Profile" {
		t.Errorf("expected profile name 'Custom Profile', got '%s'", retrieved.Name)
REDACTED
	if !retrieved.EnableGREASE {
		t.Error("expected EnableGREASE to be true")
REDACTED
REDACTED

func TestGetProfile(t *testing.T) {
	r := NewRegistry()

	// Get existing profile
	profile := r.GetProfile(DefaultProfileName)
	if profile == nil {
		t.Error("expected default profile to exist")
REDACTED

	// Get non-existing profile
	nonExistent := r.GetProfile("nonexistent")
	if nonExistent != nil {
		t.Error("expected nil for non-existent profile")
REDACTED
REDACTED

func TestGetProfileByAccountID(t *testing.T) {
	r := NewRegistry()

	// With only default profile, all account IDs should return the same profile
	for i := int64(0); i < 10; i++ {
		profile := r.GetProfileByAccountID(i)
		if profile == nil {
			t.Errorf("expected profile for account %d, got nil", i)
	REDACTED
REDACTED

	// Add more profiles
	r.RegisterProfile("profile_a", &Profile{Name: "Profile A"REDACTED)
	r.RegisterProfile("profile_b", &Profile{Name: "Profile B"REDACTED)

	// Now we have 3 profiles: claude_cli_v2, profile_a, profile_b
	// Names are sorted, so order is: claude_cli_v2, profile_a, profile_b
	expectedOrder := []string{DefaultProfileName, "profile_a", "profile_b"REDACTED
	names := r.ProfileNames()
	for i, name := range expectedOrder {
		if names[i] != name {
			t.Errorf("expected name at index %d to be %s, got %s", i, name, names[i])
	REDACTED
REDACTED

	// Test modulo selection
	// Account ID 0 % 3 = 0 -> claude_cli_v2
	// Account ID 1 % 3 = 1 -> profile_a
	// Account ID 2 % 3 = 2 -> profile_b
	// Account ID 3 % 3 = 0 -> claude_cli_v2
	testCases := []struct {
		accountID    int64
		expectedName string
REDACTED{
		{0, "Claude CLI 2.x (Node.js 20.x + OpenSSL 3.x)"REDACTED,
		{1, "Profile A"REDACTED,
		{2, "Profile B"REDACTED,
		{3, "Claude CLI 2.x (Node.js 20.x + OpenSSL 3.x)"REDACTED,
		{4, "Profile A"REDACTED,
		{5, "Profile B"REDACTED,
		{100, "Profile A"REDACTED, // 100 % 3 = 1
		{-1, "Profile A"REDACTED,  // |-1| % 3 = 1
		{-3, "Claude CLI 2.x (Node.js 20.x + OpenSSL 3.x)"REDACTED, // |-3| % 3 = 0
REDACTED

	for _, tc := range testCases {
		profile := r.GetProfileByAccountID(tc.accountID)
		if profile == nil {
			t.Errorf("expected profile for account %d, got nil", tc.accountID)
			continue
	REDACTED
		if profile.Name != tc.expectedName {
			t.Errorf("account %d: expected profile name '%s', got '%s'", tc.accountID, tc.expectedName, profile.Name)
	REDACTED
REDACTED
REDACTED

func TestNewRegistryFromConfig(t *testing.T) {
	// Test with nil config
	r := NewRegistryFromConfig(nil)
	if r.ProfileCount() != 1 {
		t.Errorf("expected 1 profile with nil config, got %d", r.ProfileCount())
REDACTED

	// Test with disabled config
	disabledCfg := &config.TLSFingerprintConfig{
		Enabled: false,
REDACTED
	r = NewRegistryFromConfig(disabledCfg)
	if r.ProfileCount() != 1 {
		t.Errorf("expected 1 profile with disabled config, got %d", r.ProfileCount())
REDACTED

	// Test with enabled config and custom profiles
	enabledCfg := &config.TLSFingerprintConfig{
		Enabled: true,
		Profiles: map[string]config.TLSProfileConfig{
			"custom1": {
				Name:         "Custom Profile 1",
				EnableGREASE: true,
		REDACTED,
			"custom2": {
				Name:         "Custom Profile 2",
				EnableGREASE: false,
		REDACTED,
	REDACTED,
REDACTED
	r = NewRegistryFromConfig(enabledCfg)

	// Should have 3 profiles: default + 2 custom
	if r.ProfileCount() != 3 {
		t.Errorf("expected 3 profiles, got %d", r.ProfileCount())
REDACTED

	// Check custom profiles exist
	custom1 := r.GetProfile("custom1")
	if custom1 == nil || custom1.Name != "Custom Profile 1" {
		t.Error("expected custom1 profile to exist with correct name")
REDACTED
	custom2 := r.GetProfile("custom2")
	if custom2 == nil || custom2.Name != "Custom Profile 2" {
		t.Error("expected custom2 profile to exist with correct name")
REDACTED
REDACTED

func TestProfileNames(t *testing.T) {
	r := NewRegistry()

	// Add profiles in non-alphabetical order
	r.RegisterProfile("zebra", &Profile{Name: "Zebra"REDACTED)
	r.RegisterProfile("alpha", &Profile{Name: "Alpha"REDACTED)
	r.RegisterProfile("beta", &Profile{Name: "Beta"REDACTED)

	names := r.ProfileNames()

	// Should be sorted alphabetically
	expected := []string{"alpha", "beta", DefaultProfileName, "zebra"REDACTED
	if len(names) != len(expected) {
		t.Errorf("expected %d names, got %d", len(expected), len(names))
REDACTED
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected name at index %d to be %s, got %s", i, name, names[i])
	REDACTED
REDACTED

	// Test that returned slice is a copy (modifying it shouldn't affect registry)
	names[0] = "modified"
	originalNames := r.ProfileNames()
	if originalNames[0] == "modified" {
		t.Error("modifying returned slice should not affect registry")
REDACTED
REDACTED

func TestConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	// Run concurrent reads and writes
	done := make(chan bool)

	// Writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				r.RegisterProfile("concurrent"+string(rune('0'+id)), &Profile{Name: "Concurrent"REDACTED)
		REDACTED
			done <- true
	REDACTED(i)
REDACTED

	// Readers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				_ = r.ProfileCount()
				_ = r.ProfileNames()
				_ = r.GetProfileByAccountID(int64(id * j))
				_ = r.GetProfile(DefaultProfileName)
		REDACTED
			done <- true
	REDACTED(i)
REDACTED

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
REDACTED

	// Test should pass without data races (run with -race flag)
REDACTED
