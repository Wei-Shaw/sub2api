//go:build unit

package service

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func timeConfig(periods ...ChannelTimePricingPeriod) *ChannelTimePricing {
	return &ChannelTimePricing{Timezone: "Asia/Shanghai", Periods: periodsREDACTED
REDACTED

func onePeriod() []ChannelTimePricingPeriod {
	return []ChannelTimePricingPeriod{{StartTime: "09:00", EndTime: "12:00", Multiplier: 2REDACTEDREDACTED
REDACTED

func TestValidateChannelTimePricing(t *testing.T) {
	tests := []struct {
		name    string
		config  *ChannelTimePricing
		wantErr string
REDACTED{
		{name: "nil disabled", config: nilREDACTED,
		{name: "empty disabled", config: &ChannelTimePricing{Timezone: "Asia/Shanghai"REDACTEDREDACTED,
		{name: "adjacent", config: timeConfig(
			ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 2REDACTED,
			ChannelTimePricingPeriod{StartTime: "12:00", EndTime: "14:00", Multiplier: 1.5REDACTED)REDACTED,
		{name: "midnight split", config: timeConfig(
			ChannelTimePricingPeriod{StartTime: "22:00", EndTime: "00:00", Multiplier: 2REDACTED,
			ChannelTimePricingPeriod{StartTime: "00:00", EndTime: "02:00", Multiplier: 2REDACTED)REDACTED,
		{name: "second precision", config: timeConfig(
			ChannelTimePricingPeriod{StartTime: "09:00:00", EndTime: "12:00:00", Multiplier: 2REDACTED,
			ChannelTimePricingPeriod{StartTime: "14:00:00", EndTime: "18:00:00", Multiplier: 2REDACTED)REDACTED,
		{name: "second precision overlap", config: timeConfig(
			ChannelTimePricingPeriod{StartTime: "09:00:00", EndTime: "12:00:00", Multiplier: 2REDACTED,
			ChannelTimePricingPeriod{StartTime: "11:59:59", EndTime: "14:00:00", Multiplier: 2REDACTED), wantErr: "overlap"REDACTED,
		{name: "empty timezone", config: &ChannelTimePricing{Periods: onePeriod()REDACTED, wantErr: "timezone"REDACTED,
		{name: "whitespace timezone", config: &ChannelTimePricing{Timezone: "  ", Periods: onePeriod()REDACTED, wantErr: "timezone"REDACTED,
		{name: "timezone", config: &ChannelTimePricing{Timezone: "UTC+8", Periods: onePeriod()REDACTED, wantErr: "timezone"REDACTED,
		{name: "format", config: timeConfig(ChannelTimePricingPeriod{StartTime: "9:00", EndTime: "12:00", Multiplier: 2REDACTED), wantErr: "HH:mm"REDACTED,
		{name: "equal midnight", config: timeConfig(ChannelTimePricingPeriod{StartTime: "00:00", EndTime: "00:00", Multiplier: 2REDACTED), wantErr: "before"REDACTED,
		{name: "cross midnight", config: timeConfig(ChannelTimePricingPeriod{StartTime: "22:00", EndTime: "02:00", Multiplier: 2REDACTED), wantErr: "before"REDACTED,
		{name: "overlap", config: timeConfig(
			ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 2REDACTED,
			ChannelTimePricingPeriod{StartTime: "11:59", EndTime: "14:00", Multiplier: 2REDACTED), wantErr: "overlap"REDACTED,
		{name: "zero", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 0REDACTED), wantErr: "greater than 0"REDACTED,
		{name: "minimum positive", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 0.01REDACTED)REDACTED,
		{name: "tiny positive", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 1e-12REDACTED), wantErr: "at least 0.01"REDACTED,
		{name: "below minimum", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 0.001REDACTED), wantErr: "at least 0.01"REDACTED,
		{name: "three decimals", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 1.001REDACTED), wantErr: "decimal"REDACTED,
		{name: "scaled overflow", config: timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: math.MaxFloat64REDACTED), wantErr: "finite"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChannelTimePricing(tt.config)
			if tt.wantErr == "" {
			REDACTED
				return
		REDACTED
		REDACTED
			require.True(t, strings.Contains(err.Error(), tt.wantErr), "error %q does not contain %q", err, tt.wantErr)
	REDACTED)
REDACTED
REDACTED

func TestChannelTimePricingMultiplierAt(t *testing.T) {
	config := timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 2REDACTED)
	tests := []struct {
		name string
		at   time.Time
		want float64
REDACTED{
		{name: "Shanghai 08:59", at: time.Date(2026, 6, 29, 0, 59, 0, 0, time.UTC), want: 1REDACTED,
		{name: "Shanghai 09:00", at: time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC), want: 2REDACTED,
		{name: "Shanghai 11:59", at: time.Date(2026, 6, 29, 3, 59, 0, 0, time.UTC), want: 2REDACTED,
		{name: "Shanghai 12:00", at: time.Date(2026, 6, 29, 4, 0, 0, 0, time.UTC), want: 1REDACTED,
		{name: "Shanghai 14:00", at: time.Date(2026, 6, 29, 6, 0, 0, 0, time.UTC), want: 1REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, config.MultiplierAt(tt.at))
	REDACTED)
REDACTED

	newYork := &ChannelTimePricing{Timezone: "America/New_York", Periods: onePeriod()REDACTED
	at := time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC)
	require.Equal(t, 1.0, config.MultiplierAt(at))
	require.Equal(t, 2.0, newYork.MultiplierAt(at))
REDACTED

func TestChannelTimePricingMultiplierAtSecondPrecision(t *testing.T) {
	config := timeConfig(ChannelTimePricingPeriod{StartTime: "09:00:30", EndTime: "09:00:45", Multiplier: 2REDACTED)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
REDACTED

	tests := []struct {
		name string
		at   time.Time
		want float64
REDACTED{
		{name: "before", at: time.Date(2026, 6, 29, 9, 0, 29, 0, shanghai), want: 1REDACTED,
		{name: "start", at: time.Date(2026, 6, 29, 9, 0, 30, 0, shanghai), want: 2REDACTED,
		{name: "last matching second", at: time.Date(2026, 6, 29, 9, 0, 44, 999_999_999, shanghai), want: 2REDACTED,
		{name: "end", at: time.Date(2026, 6, 29, 9, 0, 45, 0, shanghai), want: 1REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, config.MultiplierAt(tt.at))
	REDACTED)
REDACTED
REDACTED

func TestChannelTimePricingMultiplierAtMidnightSplit(t *testing.T) {
	config := timeConfig(
		ChannelTimePricingPeriod{StartTime: "22:00", EndTime: "00:00", Multiplier: 2REDACTED,
		ChannelTimePricingPeriod{StartTime: "00:00", EndTime: "02:00", Multiplier: 3REDACTED,
	)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
REDACTED
	tests := []struct {
		name string
		at   time.Time
		want float64
REDACTED{
		{name: "23:59", at: time.Date(2026, 6, 29, 23, 59, 0, 0, shanghai), want: 2REDACTED,
		{name: "next day 00:00", at: time.Date(2026, 6, 30, 0, 0, 0, 0, shanghai), want: 3REDACTED,
		{name: "02:00", at: time.Date(2026, 6, 30, 2, 0, 0, 0, shanghai), want: 1REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, config.MultiplierAt(tt.at))
	REDACTED)
REDACTED
REDACTED

func TestChannelTimePricingMultiplierAtDegradesForInvalidConfigurations(t *testing.T) {
	var nilConfig *ChannelTimePricing
	zeroTime := time.Time{REDACTED
	validAt := time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC)

	require.Equal(t, 1.0, nilConfig.MultiplierAt(validAt))
	require.Equal(t, 1.0, timeConfig().MultiplierAt(validAt))
	require.Equal(t, 1.0, timeConfig(ChannelTimePricingPeriod{StartTime: "09:00", EndTime: "12:00", Multiplier: 2REDACTED).MultiplierAt(zeroTime))
	require.Equal(t, 1.0, (&ChannelTimePricing{Periods: onePeriod()REDACTED).MultiplierAt(validAt))
	require.Equal(t, 1.0, (&ChannelTimePricing{Timezone: "  ", Periods: onePeriod()REDACTED).MultiplierAt(validAt))
	require.Equal(t, 1.0, (&ChannelTimePricing{Timezone: "UTC+8", Periods: onePeriod()REDACTED).MultiplierAt(validAt))
	require.Equal(t, 1.0, timeConfig(ChannelTimePricingPeriod{StartTime: "22:00", EndTime: "02:00", Multiplier: 2REDACTED).MultiplierAt(validAt))
REDACTED

func TestChannelTimePricingRejectsLocalTimezone(t *testing.T) {
	config := &ChannelTimePricing{Timezone: "Local", Periods: onePeriod()REDACTED

	err := validateChannelTimePricing(config)
REDACTED
	require.Contains(t, err.Error(), "timezone")
	require.Equal(t, 1.0, config.MultiplierAt(time.Date(2026, 6, 29, 1, 0, 0, 0, time.UTC)))
REDACTED
