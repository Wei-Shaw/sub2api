package service

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

var channelTimePricingLocations sync.Map

type parsedChannelTimePeriod struct {
	start      int
	end        int
	multiplier float64
REDACTED

// validateChannelTimePricing 校验分时倍率配置。nil 或空 periods 表示未启用。
func validateChannelTimePricing(config *ChannelTimePricing) error {
	if config == nil || len(config.Periods) == 0 {
		return nil
REDACTED
	if _, err := loadChannelTimePricingLocation(config.Timezone); err != nil {
		return fmt.Errorf("timezone: %w", err)
REDACTED
	_, err := parseChannelTimePeriods(config.Periods)
	return err
REDACTED

// MultiplierAt 返回 at 对应的分时倍率。无配置或脏配置均安全降级为 1。
func (config *ChannelTimePricing) MultiplierAt(at time.Time) float64 {
	if config == nil || len(config.Periods) == 0 || at.IsZero() {
		return 1.0
REDACTED
	if err := validateChannelTimePricing(config); err != nil {
		return 1.0
REDACTED
	location, err := loadChannelTimePricingLocation(config.Timezone)
	if err != nil {
		return 1.0
REDACTED
	periods, err := parseChannelTimePeriods(config.Periods)
	if err != nil {
		return 1.0
REDACTED

	local := at.In(location)
	second := local.Hour()*60*60 + local.Minute()*60 + local.Second()
	for _, period := range periods {
		if second >= period.start && second < period.end {
			return period.multiplier
	REDACTED
REDACTED
	return 1.0
REDACTED

func loadChannelTimePricingLocation(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("timezone is required")
REDACTED
	if name == "Local" {
		return nil, fmt.Errorf("local is not a supported timezone")
REDACTED
	if cached, ok := channelTimePricingLocations.Load(name); ok {
		location, valid := cached.(*time.Location)
		if valid && location != nil {
			return location, nil
	REDACTED
		channelTimePricingLocations.Delete(name)
REDACTED
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
REDACTED
	actual, _ := channelTimePricingLocations.LoadOrStore(name, location)
	actualLocation, ok := actual.(*time.Location)
	if !ok || actualLocation == nil {
		return nil, fmt.Errorf("invalid cached timezone %q", name)
REDACTED
	return actualLocation, nil
REDACTED

func parseChannelTime(value string, end bool) (int, error) {
	if end && (value == "00:00" || value == "00:00:00") {
		return 24 * 60 * 60, nil
REDACTED
	layout := "15:04:05"
	if len(value) == len("15:04") {
		layout = "15:04"
REDACTED
	parsed, err := time.Parse(layout, value)
	if err != nil || parsed.Format(layout) != value {
		return 0, fmt.Errorf("time %q must use HH:mm or HH:mm:ss format", value)
REDACTED
	return parsed.Hour()*60*60 + parsed.Minute()*60 + parsed.Second(), nil
REDACTED

func parseChannelTimePeriods(periods []ChannelTimePricingPeriod) ([]parsedChannelTimePeriod, error) {
	parsed := make([]parsedChannelTimePeriod, 0, len(periods))
	for _, period := range periods {
		if math.IsNaN(period.Multiplier) || math.IsInf(period.Multiplier, 0) || period.Multiplier <= 0 {
			return nil, fmt.Errorf("multiplier must be finite and greater than 0")
	REDACTED
		if period.Multiplier < 0.01 {
			return nil, fmt.Errorf("multiplier must be at least 0.01")
	REDACTED
		scaled := period.Multiplier * 100
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return nil, fmt.Errorf("multiplier must remain finite when scaled")
	REDACTED
		if math.Abs(scaled-math.Round(scaled)) > 1e-9 {
			return nil, fmt.Errorf("multiplier must have at most two decimal places")
	REDACTED

		start, err := parseChannelTime(period.StartTime, false)
		if err != nil {
			return nil, err
	REDACTED
		end, err := parseChannelTime(period.EndTime, true)
		if err != nil {
			return nil, err
	REDACTED
		if period.StartTime == period.EndTime || start >= end {
			return nil, fmt.Errorf("start time must be before end time")
	REDACTED
		parsed = append(parsed, parsedChannelTimePeriod{start: start, end: end, multiplier: period.MultiplierREDACTED)
REDACTED

	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].start < parsed[j].start
REDACTED)
	for i := 1; i < len(parsed); i++ {
		if parsed[i].start < parsed[i-1].end {
			return nil, fmt.Errorf("time pricing periods overlap")
	REDACTED
REDACTED
	return parsed, nil
REDACTED
