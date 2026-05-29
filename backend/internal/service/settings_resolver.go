package service

// Scope identifies which configuration level provided a resolved value.
type Scope int

const (
	ScopeSystem  Scope = iota // system-wide default
	ScopeGroup                // account-group override
	ScopeAccount              // per-account override
)

func (s Scope) String() string {
	switch s {
	case ScopeSystem:
		return "system"
	case ScopeGroup:
		return "group"
	case ScopeAccount:
		return "account"
	default:
		return "unknown"
	}
}

// ResolveInt resolves an integer setting using DirectionOverride semantics:
// the most specific non-zero value wins (account > group > system).
// Zero means "not configured at this level; inherit from next".
func ResolveInt(accountVal, groupVal, systemVal int) (int, Scope) {
	if accountVal > 0 {
		return accountVal, ScopeAccount
	}
	if groupVal > 0 {
		return groupVal, ScopeGroup
	}
	return systemVal, ScopeSystem
}

// ResolveIntCeiling resolves using DirectionCeiling semantics:
// the most restrictive (smallest) non-zero value wins.
// Zero means "no limit at this level".
func ResolveIntCeiling(accountVal, groupVal, systemVal int) (int, Scope) {
	result, scope := systemVal, ScopeSystem
	if groupVal > 0 && (result == 0 || groupVal < result) {
		result, scope = groupVal, ScopeGroup
	}
	if accountVal > 0 && (result == 0 || accountVal < result) {
		result, scope = accountVal, ScopeAccount
	}
	return result, scope
}

// ResolveBool resolves a *bool setting using DirectionOverride semantics.
// nil means "not configured at this level; inherit from next".
func ResolveBool(accountVal, groupVal *bool, systemVal bool) (bool, Scope) {
	if accountVal != nil {
		return *accountVal, ScopeAccount
	}
	if groupVal != nil {
		return *groupVal, ScopeGroup
	}
	return systemVal, ScopeSystem
}

// ResolveBoolCeiling resolves a *bool using DirectionCeiling semantics:
// false (more restrictive) wins over true. When multiple levels are false,
// the most specific level (account > group > system) is reported as source.
func ResolveBoolCeiling(accountVal, groupVal *bool, systemVal bool) (bool, Scope) {
	result, scope := systemVal, ScopeSystem
	if groupVal != nil && !*groupVal {
		result, scope = false, ScopeGroup
	}
	if accountVal != nil && !*accountVal {
		result, scope = false, ScopeAccount
	}
	return result, scope
}
