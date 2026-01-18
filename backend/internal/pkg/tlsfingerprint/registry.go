// Package tlsfingerprint provides TLS fingerprint simulation for HTTP clients.
package tlsfingerprint

import (
	"sort"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// DefaultProfileName is the name of the built-in Claude CLI profile.
const DefaultProfileName = "claude_cli_v2"

// Registry manages TLS fingerprint profiles.
// It holds a collection of profiles that can be used for TLS fingerprint simulation.
// Profiles are selected based on account ID using modulo operation.
type Registry struct {
	mu           sync.RWMutex
	profiles     map[string]*Profile
	profileNames []string // Sorted list of profile names for deterministic selection
REDACTED

// NewRegistry creates a new TLS fingerprint profile registry.
// It initializes with the built-in default profile.
func NewRegistry() *Registry {
	r := &Registry{
		profiles:     make(map[string]*Profile),
		profileNames: make([]string, 0),
REDACTED

	// Register the built-in default profile
	r.registerBuiltinProfile()

	return r
REDACTED

// NewRegistryFromConfig creates a new registry and loads profiles from config.
// If the config has custom profiles defined, they will be merged with the built-in default.
func NewRegistryFromConfig(cfg *config.TLSFingerprintConfig) *Registry {
	r := NewRegistry()

	if cfg == nil || !cfg.Enabled {
		debugLog("[TLS Registry] TLS fingerprint disabled or no config, using default profile only")
		return r
REDACTED

	// Load custom profiles from config
	for name, profileCfg := range cfg.Profiles {
		profile := &Profile{
			Name:         profileCfg.Name,
			EnableGREASE: profileCfg.EnableGREASE,
			CipherSuites: profileCfg.CipherSuites,
			Curves:       profileCfg.Curves,
			PointFormats: profileCfg.PointFormats,
	REDACTED

		// If the profile has empty values, they will use defaults in dialer
		r.RegisterProfile(name, profile)
		debugLog("[TLS Registry] Loaded custom profile: %s (%s)", name, profileCfg.Name)
REDACTED

	debugLog("[TLS Registry] Initialized with %d profiles: %v", len(r.profileNames), r.profileNames)
	return r
REDACTED

// registerBuiltinProfile adds the default Claude CLI profile to the registry.
func (r *Registry) registerBuiltinProfile() {
	defaultProfile := &Profile{
		Name:         "Claude CLI 2.x (Node.js 20.x + OpenSSL 3.x)",
		EnableGREASE: false, // Node.js does not use GREASE
		// Empty slices will cause dialer to use built-in defaults
		CipherSuites: nil,
		Curves:       nil,
		PointFormats: nil,
REDACTED
	r.RegisterProfile(DefaultProfileName, defaultProfile)
REDACTED

// RegisterProfile adds or updates a profile in the registry.
func (r *Registry) RegisterProfile(name string, profile *Profile) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if this is a new profile
	_, exists := r.profiles[name]
	r.profiles[name] = profile

	if !exists {
		r.profileNames = append(r.profileNames, name)
		// Keep names sorted for deterministic selection
		sort.Strings(r.profileNames)
REDACTED
REDACTED

// GetProfile returns a profile by name.
// Returns nil if the profile does not exist.
func (r *Registry) GetProfile(name string) *Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.profiles[name]
REDACTED

// GetDefaultProfile returns the built-in default profile.
func (r *Registry) GetDefaultProfile() *Profile {
	return r.GetProfile(DefaultProfileName)
REDACTED

// GetProfileByAccountID returns a profile for the given account ID.
// The profile is selected using: profileNames[accountID % len(profiles)]
// This ensures deterministic profile assignment for each account.
func (r *Registry) GetProfileByAccountID(accountID int64) *Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.profileNames) == 0 {
		return nil
REDACTED

	// Use modulo to select profile index
	// Use absolute value to handle negative IDs (though unlikely)
	idx := accountID
	if idx < 0 {
		idx = -idx
REDACTED
	selectedIndex := int(idx % int64(len(r.profileNames)))
	selectedName := r.profileNames[selectedIndex]

	return r.profiles[selectedName]
REDACTED

// ProfileCount returns the number of registered profiles.
func (r *Registry) ProfileCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.profiles)
REDACTED

// ProfileNames returns a sorted list of all registered profile names.
func (r *Registry) ProfileNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent modification
	names := make([]string, len(r.profileNames))
	copy(names, r.profileNames)
	return names
REDACTED

// Global registry instance for convenience
var globalRegistry *Registry
var globalRegistryOnce sync.Once

// GlobalRegistry returns the global TLS fingerprint registry.
// The registry is lazily initialized with the default profile.
func GlobalRegistry() *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistry()
REDACTED)
	return globalRegistry
REDACTED

// InitGlobalRegistry initializes the global registry with configuration.
// This should be called during application startup.
// It is safe to call multiple times; subsequent calls will update the registry.
func InitGlobalRegistry(cfg *config.TLSFingerprintConfig) *Registry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewRegistryFromConfig(cfg)
REDACTED)
	return globalRegistry
REDACTED
