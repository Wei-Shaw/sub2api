//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePlanRequired_AllValid(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", nil)
REDACTED
REDACTED

func TestValidatePlanRequired_EmptyName(t *testing.T) {
	err := validatePlanRequired("", 1, 9.99, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "plan name")
REDACTED

func TestValidatePlanRequired_WhitespaceName(t *testing.T) {
	err := validatePlanRequired("   ", 1, 9.99, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "plan name")
REDACTED

func TestValidatePlanRequired_ZeroGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", 0, 9.99, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "group")
REDACTED

func TestValidatePlanRequired_NegativeGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", -1, 9.99, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "group")
REDACTED

func TestValidatePlanRequired_ZeroPrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 0, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "price")
REDACTED

func TestValidatePlanRequired_NegativePrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, -5, 30, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "price")
REDACTED

func TestValidatePlanRequired_ZeroValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 0, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "validity days")
REDACTED

func TestValidatePlanRequired_NegativeValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, -7, "days", nil)
REDACTED
	require.Contains(t, err.Error(), "validity days")
REDACTED

func TestValidatePlanRequired_EmptyValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "", nil)
REDACTED
	require.Contains(t, err.Error(), "validity unit")
REDACTED

func TestValidatePlanRequired_WhitespaceValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "   ", nil)
REDACTED
	require.Contains(t, err.Error(), "validity unit")
REDACTED

func TestValidatePlanRequired_NameValidatedFirst(t *testing.T) {
	err := validatePlanRequired("", 0, 0, 0, "", nil)
REDACTED
	require.Contains(t, err.Error(), "plan name")
REDACTED

func TestValidatePlanRequired_TrimmedValidName(t *testing.T) {
	err := validatePlanRequired("  Pro  ", 1, 9.99, 30, "days", nil)
REDACTED
REDACTED

func TestValidatePlanRequired_NegativeOriginalPrice(t *testing.T) {
	neg := -10.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &neg)
REDACTED
	require.Contains(t, err.Error(), "original price")
REDACTED

func TestValidatePlanRequired_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &zero)
REDACTED
REDACTED

func TestValidatePlanRequired_ValidOriginalPrice(t *testing.T) {
	op := 19.99
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &op)
REDACTED
REDACTED

// --- validatePlanPatch tests ---

func TestValidatePlanPatch_NegativeOriginalPrice(t *testing.T) {
	neg := -5.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &negREDACTED)
REDACTED
	require.Contains(t, err.Error(), "original price")
REDACTED

func TestValidatePlanPatch_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &zeroREDACTED)
REDACTED
REDACTED

func TestValidatePlanPatch_ValidOriginalPrice(t *testing.T) {
	op := 29.99
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &opREDACTED)
REDACTED
REDACTED

func TestValidatePlanPatch_NilOriginalPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: nilREDACTED)
REDACTED
REDACTED

// --- validatePlanPatch: other fields ---

func ptrStr(s string) *string     { return &s REDACTED
func ptrInt(i int) *int           { return &i REDACTED
func ptrInt64(i int64) *int64     { return &i REDACTED
func ptrFloat(f float64) *float64 { return &f REDACTED

func TestValidatePlanPatch_EmptyName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("")REDACTED)
REDACTED
	require.Contains(t, err.Error(), "plan name")
REDACTED

func TestValidatePlanPatch_ValidName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("Basic")REDACTED)
REDACTED
REDACTED

func TestValidatePlanPatch_ZeroGroupID(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{GroupID: ptrInt64(0)REDACTED)
REDACTED
	require.Contains(t, err.Error(), "group")
REDACTED

func TestValidatePlanPatch_NegativePrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(-1)REDACTED)
REDACTED
	require.Contains(t, err.Error(), "price")
REDACTED

func TestValidatePlanPatch_ZeroPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(0)REDACTED)
REDACTED
	require.Contains(t, err.Error(), "price")
REDACTED

func TestValidatePlanPatch_ValidPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(9.99)REDACTED)
REDACTED
REDACTED

func TestValidatePlanPatch_ZeroValidityDays(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityDays: ptrInt(0)REDACTED)
REDACTED
	require.Contains(t, err.Error(), "validity days")
REDACTED

func TestValidatePlanPatch_EmptyValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("")REDACTED)
REDACTED
	require.Contains(t, err.Error(), "validity unit")
REDACTED

func TestValidatePlanPatch_ValidValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("days")REDACTED)
REDACTED
REDACTED

func TestValidatePlanPatch_AllNil(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{REDACTED)
REDACTED
REDACTED

// --- normalizePlanCurrency tests ---
// Empty must stay empty (not coerced to the default payment currency),
// so existing plans keep rendering without any currency label.

func TestNormalizePlanCurrency_EmptyKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("")
REDACTED
	require.Equal(t, "", currency)
REDACTED

func TestNormalizePlanCurrency_WhitespaceKeepsEmpty(t *testing.T) {
	currency, err := normalizePlanCurrency("   ")
REDACTED
	require.Equal(t, "", currency)
REDACTED

func TestNormalizePlanCurrency_LowercaseNormalized(t *testing.T) {
	currency, err := normalizePlanCurrency("nzd")
REDACTED
	require.Equal(t, "NZD", currency)
REDACTED

func TestNormalizePlanCurrency_ValidUppercase(t *testing.T) {
	currency, err := normalizePlanCurrency("USD")
REDACTED
	require.Equal(t, "USD", currency)
REDACTED

func TestNormalizePlanCurrency_TooShort(t *testing.T) {
	_, err := normalizePlanCurrency("NZ")
REDACTED
	require.Contains(t, err.Error(), "currency")
REDACTED

func TestNormalizePlanCurrency_TooLong(t *testing.T) {
	_, err := normalizePlanCurrency("NZDD")
REDACTED
	require.Contains(t, err.Error(), "currency")
REDACTED

func TestNormalizePlanCurrency_NonLetter(t *testing.T) {
	_, err := normalizePlanCurrency("N2D")
REDACTED
	require.Contains(t, err.Error(), "currency")
REDACTED
