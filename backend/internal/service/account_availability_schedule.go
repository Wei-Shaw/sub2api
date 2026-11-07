package service

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const (
	availabilityScheduleExtraKey = "availability_schedule"
	availabilityScheduleMaxRules = 20

	availabilityScheduleKindDaily  = "daily"
	availabilityScheduleKindWeekly = "weekly"

	availabilityScheduleActionEnable  = "enable"
	availabilityScheduleActionDisable = "disable"
)

// AvailabilitySchedule controls recurring daily/weekly windows that force an
// account into or out of the schedulable pool. Stored under accounts.extra.
type AvailabilitySchedule struct {
	Enabled  bool                      `json:"enabled"`
	Timezone string                    `json:"timezone,omitempty"`
	Rules    []AvailabilityScheduleRule `json:"rules"`
}

// AvailabilityScheduleRule is evaluated in array order; first match wins.
type AvailabilityScheduleRule struct {
	ID       string `json:"id,omitempty"`
	Kind     string `json:"kind"`               // daily | weekly
	Weekdays []int  `json:"weekdays,omitempty"` // Mon=1 … Sun=7
	Start    string `json:"start"`              // HH:MM
	End      string `json:"end"`                // HH:MM; end<=start means overnight (or all-day when equal)
	Action   string `json:"action"`             // enable | disable
}

// GetAvailabilitySchedule parses extra.availability_schedule.
// Invalid payloads return nil (schedule treated as disabled) and log a warning.
func (a *Account) GetAvailabilitySchedule() *AvailabilitySchedule {
	if a == nil || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[availabilityScheduleExtraKey]
	if !ok || raw == nil {
		return nil
	}
	schedule, err := parseAvailabilitySchedule(raw)
	if err != nil {
		slog.Warn("account.availability_schedule_invalid",
			"account_id", a.ID,
			"error", err.Error(),
		)
		return nil
	}
	return schedule
}

// appliesAvailabilitySchedule returns whether the schedule overrides the result
// and the forced schedulable value when it does. ok=false means "no override".
func (a *Account) appliesAvailabilitySchedule(now time.Time) (forced bool, ok bool) {
	schedule := a.GetAvailabilitySchedule()
	if schedule == nil || !schedule.Enabled || len(schedule.Rules) == 0 {
		return false, false
	}
	loc := resolveAvailabilityLocation(schedule.Timezone)
	local := now.In(loc)
	for _, rule := range schedule.Rules {
		if !availabilityRuleMatches(rule, local) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rule.Action)) {
		case availabilityScheduleActionDisable:
			return false, true
		case availabilityScheduleActionEnable:
			// Caller already required active+manual schedulable; enable cannot bypass that.
			return true, true
		default:
			continue
		}
	}
	return false, false
}

func resolveAvailabilityLocation(tzName string) *time.Location {
	tzName = strings.TrimSpace(tzName)
	if tzName != "" {
		if loc, err := time.LoadLocation(tzName); err == nil {
			return loc
		}
		slog.Warn("account.availability_schedule_timezone_fallback", "timezone", tzName)
	}
	return timezone.Location()
}

func availabilityRuleMatches(rule AvailabilityScheduleRule, local time.Time) bool {
	kind := strings.ToLower(strings.TrimSpace(rule.Kind))
	switch kind {
	case availabilityScheduleKindWeekly:
		weekday := weekdayMon1Sun7(local)
		found := false
		for _, d := range rule.Weekdays {
			if d == weekday {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	case availabilityScheduleKindDaily:
		// every day
	default:
		return false
	}
	startMin, ok1 := parseHHMMToMinutes(rule.Start)
	endMin, ok2 := parseHHMMToMinutes(rule.End)
	if !ok1 || !ok2 {
		return false
	}
	nowMin := local.Hour()*60 + local.Minute()
	return minutesInWindow(nowMin, startMin, endMin)
}

func weekdayMon1Sun7(t time.Time) int {
	// time.Weekday: Sunday=0 … Saturday=6 → Mon=1 … Sun=7
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

func parseHHMMToMinutes(value string) (int, bool) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// minutesInWindow uses half-open [start, end) for same-day windows.
// When start == end, the window matches the full day.
// When end < start, the window crosses midnight: [start, 24h) U [0, end).
func minutesInWindow(now, start, end int) bool {
	if start == end {
		return true
	}
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end
}

func parseAvailabilitySchedule(raw any) (*AvailabilitySchedule, error) {
	entry, ok := raw.(map[string]any)
	if !ok || entry == nil {
		return nil, fmt.Errorf("availability_schedule must be an object")
	}
	schedule := &AvailabilitySchedule{
		Enabled:  parseAvailabilityBool(entry["enabled"]),
		Timezone: strings.TrimSpace(parseAvailabilityString(entry["timezone"])),
	}
	rawRules := entry["rules"]
	if rawRules == nil {
		return schedule, nil
	}
	arr, ok := rawRules.([]any)
	if !ok {
		return nil, fmt.Errorf("availability_schedule.rules must be an array")
	}
	if len(arr) > availabilityScheduleMaxRules {
		return nil, fmt.Errorf("availability_schedule.rules exceeds max of %d", availabilityScheduleMaxRules)
	}
	rules := make([]AvailabilityScheduleRule, 0, len(arr))
	for i, item := range arr {
		ruleMap, ok := item.(map[string]any)
		if !ok || ruleMap == nil {
			return nil, fmt.Errorf("availability_schedule.rules[%d] must be an object", i)
		}
		rule := AvailabilityScheduleRule{
			ID:       strings.TrimSpace(parseAvailabilityString(ruleMap["id"])),
			Kind:     strings.ToLower(strings.TrimSpace(parseAvailabilityString(ruleMap["kind"]))),
			Weekdays: parseAvailabilityWeekdays(ruleMap["weekdays"]),
			Start:    strings.TrimSpace(parseAvailabilityString(ruleMap["start"])),
			End:      strings.TrimSpace(parseAvailabilityString(ruleMap["end"])),
			Action:   strings.ToLower(strings.TrimSpace(parseAvailabilityString(ruleMap["action"]))),
		}
		if err := validateAvailabilityScheduleRule(rule, i); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	schedule.Rules = rules
	if schedule.Timezone != "" {
		if _, err := time.LoadLocation(schedule.Timezone); err != nil {
			return nil, fmt.Errorf("availability_schedule.timezone must be a valid IANA timezone name")
		}
	}
	return schedule, nil
}

func validateAvailabilityScheduleRule(rule AvailabilityScheduleRule, index int) error {
	switch rule.Kind {
	case availabilityScheduleKindDaily, availabilityScheduleKindWeekly:
	default:
		return fmt.Errorf("availability_schedule.rules[%d].kind must be daily or weekly", index)
	}
	switch rule.Action {
	case availabilityScheduleActionEnable, availabilityScheduleActionDisable:
	default:
		return fmt.Errorf("availability_schedule.rules[%d].action must be enable or disable", index)
	}
	if _, ok := parseHHMMToMinutes(rule.Start); !ok {
		return fmt.Errorf("availability_schedule.rules[%d].start must be HH:MM", index)
	}
	if _, ok := parseHHMMToMinutes(rule.End); !ok {
		return fmt.Errorf("availability_schedule.rules[%d].end must be HH:MM", index)
	}
	if rule.Kind == availabilityScheduleKindWeekly {
		if len(rule.Weekdays) == 0 {
			return fmt.Errorf("availability_schedule.rules[%d].weekdays is required for weekly rules", index)
		}
		for _, d := range rule.Weekdays {
			if d < 1 || d > 7 {
				return fmt.Errorf("availability_schedule.rules[%d].weekdays must be integers 1-7 (Mon-Sun)", index)
			}
		}
	}
	return nil
}

// ValidateAvailabilityScheduleExtra validates extra.availability_schedule when present.
func ValidateAvailabilityScheduleExtra(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	raw, exists := extra[availabilityScheduleExtraKey]
	if !exists || raw == nil {
		return nil
	}
	_, err := parseAvailabilitySchedule(raw)
	return err
}

// NormalizeAvailabilityScheduleExtra rewrites a validated schedule into a clean map shape.
func NormalizeAvailabilityScheduleExtra(extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	raw, exists := extra[availabilityScheduleExtraKey]
	if !exists {
		return extra, nil
	}
	if raw == nil {
		delete(extra, availabilityScheduleExtraKey)
		return extra, nil
	}
	schedule, err := parseAvailabilitySchedule(raw)
	if err != nil {
		return nil, err
	}
	extra[availabilityScheduleExtraKey] = scheduleToExtraMap(schedule)
	return extra, nil
}

func scheduleToExtraMap(schedule *AvailabilitySchedule) map[string]any {
	rules := make([]any, 0, len(schedule.Rules))
	for _, rule := range schedule.Rules {
		entry := map[string]any{
			"kind":   rule.Kind,
			"start":  rule.Start,
			"end":    rule.End,
			"action": rule.Action,
		}
		if rule.ID != "" {
			entry["id"] = rule.ID
		}
		if rule.Kind == availabilityScheduleKindWeekly {
			weekdays := make([]any, len(rule.Weekdays))
			for i, d := range rule.Weekdays {
				weekdays[i] = d
			}
			entry["weekdays"] = weekdays
		}
		rules = append(rules, entry)
	}
	out := map[string]any{
		"enabled": schedule.Enabled,
		"rules":   rules,
	}
	if schedule.Timezone != "" {
		out["timezone"] = schedule.Timezone
	}
	return out
}

func parseAvailabilityBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(t))
		return err == nil && b
	default:
		return false
	}
}

func parseAvailabilityString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func parseAvailabilityWeekdays(v any) []int {
	arr, ok := v.([]any)
	if !ok {
		if ints, ok := v.([]int); ok {
			out := make([]int, 0, len(ints))
			out = append(out, ints...)
			return out
		}
		return nil
	}
	out := make([]int, 0, len(arr))
	seen := make(map[int]struct{}, len(arr))
	for _, item := range arr {
		d := parseAvailabilityInt(item)
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

func parseAvailabilityInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}
