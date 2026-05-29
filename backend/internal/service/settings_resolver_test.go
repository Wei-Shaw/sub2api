package service

import "testing"

func resolverBoolPtr(b bool) *bool { return &b }

func TestResolveInt(t *testing.T) {
	tests := []struct {
		name      string
		account   int
		group     int
		system    int
		wantValue int
		wantScope Scope
	}{
		{"account wins", 10, 5, 3, 10, ScopeAccount},
		{"group wins when account unset", 0, 5, 3, 5, ScopeGroup},
		{"system wins when both unset", 0, 0, 3, 3, ScopeSystem},
		{"all zero returns system zero", 0, 0, 0, 0, ScopeSystem},
		{"account beats group and system", 7, 0, 100, 7, ScopeAccount},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, scope := ResolveInt(tc.account, tc.group, tc.system)
			if got != tc.wantValue || scope != tc.wantScope {
				t.Errorf("ResolveInt(%d,%d,%d) = (%d,%v), want (%d,%v)",
					tc.account, tc.group, tc.system, got, scope, tc.wantValue, tc.wantScope)
			}
		})
	}
}

func TestResolveIntCeiling(t *testing.T) {
	tests := []struct {
		name      string
		account   int
		group     int
		system    int
		wantValue int
		wantScope Scope
	}{
		{"account most restrictive", 2, 5, 10, 2, ScopeAccount},
		{"group most restrictive", 0, 5, 10, 5, ScopeGroup},
		{"system most restrictive", 10, 0, 3, 3, ScopeSystem},
		{"system more restrictive than group", 0, 8, 5, 5, ScopeSystem},
		{"account more restrictive than system", 3, 0, 10, 3, ScopeAccount},
		{"all zero returns system zero (no limit)", 0, 0, 0, 0, ScopeSystem},
		{"only system set", 0, 0, 7, 7, ScopeSystem},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, scope := ResolveIntCeiling(tc.account, tc.group, tc.system)
			if got != tc.wantValue || scope != tc.wantScope {
				t.Errorf("ResolveIntCeiling(%d,%d,%d) = (%d,%v), want (%d,%v)",
					tc.account, tc.group, tc.system, got, scope, tc.wantValue, tc.wantScope)
			}
		})
	}
}

func TestResolveBool(t *testing.T) {
	tests := []struct {
		name      string
		account   *bool
		group     *bool
		system    bool
		wantValue bool
		wantScope Scope
	}{
		{"account true wins", resolverBoolPtr(true), resolverBoolPtr(false), false, true, ScopeAccount},
		{"account false wins over system true", resolverBoolPtr(false), nil, true, false, ScopeAccount},
		{"group wins when account nil", nil, resolverBoolPtr(true), false, true, ScopeGroup},
		{"system wins when both nil", nil, nil, true, true, ScopeSystem},
		{"all nil returns system false", nil, nil, false, false, ScopeSystem},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, scope := ResolveBool(tc.account, tc.group, tc.system)
			if got != tc.wantValue || scope != tc.wantScope {
				t.Errorf("ResolveBool = (%v,%v), want (%v,%v)", got, scope, tc.wantValue, tc.wantScope)
			}
		})
	}
}

func TestResolveBoolCeiling(t *testing.T) {
	tests := []struct {
		name      string
		account   *bool
		group     *bool
		system    bool
		wantValue bool
		wantScope Scope
	}{
		{"system false cannot be opened by account true", resolverBoolPtr(true), nil, false, false, ScopeSystem},
		{"account false beats system true", resolverBoolPtr(false), nil, true, false, ScopeAccount},
		{"group false beats system true", nil, resolverBoolPtr(false), true, false, ScopeGroup},
		{"account false beats group false (account is most specific)", resolverBoolPtr(false), resolverBoolPtr(false), true, false, ScopeAccount},
		{"all true returns true", resolverBoolPtr(true), resolverBoolPtr(true), true, true, ScopeSystem},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, scope := ResolveBoolCeiling(tc.account, tc.group, tc.system)
			if got != tc.wantValue || scope != tc.wantScope {
				t.Errorf("ResolveBoolCeiling = (%v,%v), want (%v,%v)", got, scope, tc.wantValue, tc.wantScope)
			}
		})
	}
}
