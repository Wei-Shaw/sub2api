package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultTimePricingTimezone = "Asia/Shanghai"
	minutesPerDay              = 24 * 60
	minutesPerWeek             = 7 * minutesPerDay
)

// MatchTimePricingPeriod 返回指定时刻命中的第一个时段。
// 普通时段按当前星期匹配；跨午夜时段的凌晨部分归属于前一天的开始时段。
func MatchTimePricingPeriod(config *TimePricingConfig, at time.Time) *TimePricingPeriod {
	if config == nil || !config.Enabled || len(config.Periods) == 0 {
		return nil
	}
	location, err := time.LoadLocation(normalizeTimePricingTimezone(config.Timezone))
	if err != nil {
		return nil
	}
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	weekday := int(local.Weekday())

	for i := range config.Periods {
		period := &config.Periods[i]
		start, startErr := parseTimePricingClock(period.StartTime)
		end, endErr := parseTimePricingClock(period.EndTime)
		if startErr != nil || endErr != nil || start == end {
			continue
		}
		if start < end {
			if minute >= start && minute < end && timePricingWeekdayMatches(period.Weekdays, weekday) {
				return period
			}
			continue
		}
		if minute >= start && timePricingWeekdayMatches(period.Weekdays, weekday) {
			return period
		}
		previousWeekday := (weekday + 6) % 7
		if minute < end && timePricingWeekdayMatches(period.Weekdays, previousWeekday) {
			return period
		}
	}
	return nil
}

func normalizeTimePricingTimezone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultTimePricingTimezone
	}
	return value
}

func parseTimePricingClock(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("time must use HH:mm format")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, fmt.Errorf("hour must be between 00 and 23")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("minute must be between 00 and 59")
	}
	return hour*60 + minute, nil
}

func timePricingWeekdayMatches(weekdays []int, weekday int) bool {
	if len(weekdays) == 0 {
		return true
	}
	for _, day := range weekdays {
		if day == weekday {
			return true
		}
	}
	return false
}

func validateTimePricing(pricing ChannelModelPricing) error {
	config := pricing.TimePricing
	if config == nil || !config.Enabled {
		return nil
	}
	config.Timezone = normalizeTimePricingTimezone(config.Timezone)
	if _, err := time.LoadLocation(config.Timezone); err != nil {
		return infraerrors.BadRequest("INVALID_TIME_PRICING_TIMEZONE", fmt.Sprintf("invalid time pricing timezone %q", config.Timezone))
	}
	if len(config.Periods) == 0 {
		return infraerrors.BadRequest("TIME_PRICING_PERIODS_REQUIRED", "at least one time pricing period is required")
	}

	occupied := make([]int, minutesPerWeek)
	for i := range config.Periods {
		period := &config.Periods[i]
		start, err := parseTimePricingClock(period.StartTime)
		if err != nil {
			return infraerrors.BadRequest("INVALID_TIME_PRICING_START", fmt.Sprintf("time pricing period #%d start_time: %v", i+1, err))
		}
		end, err := parseTimePricingClock(period.EndTime)
		if err != nil {
			return infraerrors.BadRequest("INVALID_TIME_PRICING_END", fmt.Sprintf("time pricing period #%d end_time: %v", i+1, err))
		}
		if start == end {
			return infraerrors.BadRequest("INVALID_TIME_PRICING_RANGE", fmt.Sprintf("time pricing period #%d start_time and end_time must differ", i+1))
		}
		if err := validateTimePricingWeekdays(period.Weekdays, i); err != nil {
			return err
		}
		if err := validateTimePricingPeriodPrices(pricing.BillingMode, period, i); err != nil {
			return err
		}
		if err := reserveTimePricingMinutes(occupied, period.Weekdays, start, end, i+1); err != nil {
			return err
		}
	}
	return nil
}

func validateTimePricingWeekdays(weekdays []int, periodIndex int) error {
	seen := make(map[int]struct{}, len(weekdays))
	for _, day := range weekdays {
		if day < 0 || day > 6 {
			return infraerrors.BadRequest("INVALID_TIME_PRICING_WEEKDAY", fmt.Sprintf("time pricing period #%d weekday must be between 0 and 6", periodIndex+1))
		}
		if _, exists := seen[day]; exists {
			return infraerrors.BadRequest("DUPLICATE_TIME_PRICING_WEEKDAY", fmt.Sprintf("time pricing period #%d contains duplicate weekday %d", periodIndex+1, day))
		}
		seen[day] = struct{}{}
	}
	return nil
}

func validateTimePricingPeriodPrices(mode BillingMode, period *TimePricingPeriod, periodIndex int) error {
	prices := []struct {
		name  string
		value *float64
	}{
		{"input_price", period.InputPrice},
		{"output_price", period.OutputPrice},
		{"cache_write_price", period.CacheWritePrice},
		{"cache_read_price", period.CacheReadPrice},
		{"image_output_price", period.ImageOutputPrice},
		{"per_request_price", period.PerRequestPrice},
	}
	for _, price := range prices {
		if price.value != nil && *price.value < 0 {
			return infraerrors.BadRequest("NEGATIVE_TIME_PRICING_PRICE", fmt.Sprintf("time pricing period #%d %s must be >= 0", periodIndex+1, price.name))
		}
	}

	if mode == BillingModePerRequest || mode == BillingModeImage {
		if period.PerRequestPrice == nil {
			return infraerrors.BadRequest("TIME_PRICING_PRICE_REQUIRED", fmt.Sprintf("time pricing period #%d requires per_request_price", periodIndex+1))
		}
		return nil
	}
	if period.InputPrice == nil && period.OutputPrice == nil &&
		period.CacheWritePrice == nil && period.CacheReadPrice == nil &&
		period.ImageOutputPrice == nil {
		return infraerrors.BadRequest("TIME_PRICING_PRICE_REQUIRED", fmt.Sprintf("time pricing period #%d requires at least one token price", periodIndex+1))
	}
	return nil
}

func reserveTimePricingMinutes(occupied []int, weekdays []int, start, end, periodNumber int) error {
	days := weekdays
	if len(days) == 0 {
		days = []int{0, 1, 2, 3, 4, 5, 6}
	}
	duration := end - start
	if duration < 0 {
		duration += minutesPerDay
	}
	for _, day := range days {
		first := day*minutesPerDay + start
		for offset := 0; offset < duration; offset++ {
			minute := (first + offset) % minutesPerWeek
			if occupied[minute] != 0 {
				return infraerrors.BadRequest(
					"OVERLAPPING_TIME_PRICING_PERIODS",
					fmt.Sprintf("time pricing periods #%d and #%d overlap", occupied[minute], periodNumber),
				)
			}
			occupied[minute] = periodNumber
		}
	}
	return nil
}

func applyTimePricingToModelPricing(pricing *ModelPricing, period *TimePricingPeriod) *ModelPricing {
	if pricing == nil || period == nil {
		return pricing
	}
	cloned := *pricing
	if period.InputPrice != nil {
		cloned.InputPricePerToken = *period.InputPrice
		cloned.InputPricePerTokenPriority = *period.InputPrice
	}
	if period.OutputPrice != nil {
		cloned.OutputPricePerToken = *period.OutputPrice
		cloned.OutputPricePerTokenPriority = *period.OutputPrice
	}
	if period.CacheWritePrice != nil {
		cloned.CacheCreationPricePerToken = *period.CacheWritePrice
		cloned.CacheCreation5mPrice = *period.CacheWritePrice
		cloned.CacheCreation1hPrice = *period.CacheWritePrice
	}
	if period.CacheReadPrice != nil {
		cloned.CacheReadPricePerToken = *period.CacheReadPrice
		cloned.CacheReadPricePerTokenPriority = *period.CacheReadPrice
	}
	if period.ImageOutputPrice != nil {
		cloned.ImageOutputPricePerToken = *period.ImageOutputPrice
		cloned.ImageOutputPriceExplicit = true
	}
	return &cloned
}

func applyTimePricingToChannelPricing(pricing *ChannelModelPricing, period *TimePricingPeriod) ChannelModelPricing {
	cloned := pricing.Clone()
	if period == nil {
		return cloned
	}
	if period.InputPrice != nil {
		cloned.InputPrice = period.InputPrice
	}
	if period.OutputPrice != nil {
		cloned.OutputPrice = period.OutputPrice
	}
	if period.CacheWritePrice != nil {
		cloned.CacheWritePrice = period.CacheWritePrice
	}
	if period.CacheReadPrice != nil {
		cloned.CacheReadPrice = period.CacheReadPrice
	}
	if period.ImageOutputPrice != nil {
		cloned.ImageOutputPrice = period.ImageOutputPrice
	}
	if period.PerRequestPrice != nil {
		cloned.PerRequestPrice = period.PerRequestPrice
	}
	return cloned
}
