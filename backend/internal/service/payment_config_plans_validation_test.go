//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePlanRequired_AllValid(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days")
	require.NoError(t, err)
}

func TestValidatePlanRequired_EmptyName(t *testing.T) {
	err := validatePlanRequired("", 1, 9.99, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_WhitespaceName(t *testing.T) {
	err := validatePlanRequired("   ", 1, 9.99, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_ZeroGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", 0, 9.99, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_NegativeGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", -1, 9.99, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_ZeroPrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 0, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_NegativePrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, -5, 30, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_ZeroValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 0, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_NegativeValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, -7, "days")
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_EmptyValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_WhitespaceValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "   ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_NameValidatedFirst(t *testing.T) {
	// When multiple fields are invalid, name should be reported first
	// (follows the order of checks in the function).
	err := validatePlanRequired("", 0, 0, 0, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_TrimmedValidName(t *testing.T) {
	// Whitespace-surrounded but non-empty name is accepted (trimmed check only
	// rejects pure whitespace).
	err := validatePlanRequired("  Pro  ", 1, 9.99, 30, "days")
	require.NoError(t, err)
}
