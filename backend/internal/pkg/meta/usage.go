package meta

import "encoding/json"

const SubscriptionUsageEventType = "response.subscription_usage"

type SubscriptionUsageWindow struct {
	ResetsAt           int64   `json:"resets_at"`
	UsedPercent        float64 `json:"used_percent"`
	WindowDurationMins int     `json:"window_duration_mins,omitempty"`
}

type SubscriptionUsage struct {
	Tier   string                   `json:"tier,omitempty"`
	Window *SubscriptionUsageWindow `json:"window,omitempty"`
	Weekly *SubscriptionUsageWindow `json:"weekly,omitempty"`
}

type subscriptionUsageEnvelope struct {
	Type         string             `json:"type"`
	Subscription *SubscriptionUsage `json:"subscription"`
}

func IsSubscriptionUsageEvent(eventType string, data []byte) bool {
	if eventType == SubscriptionUsageEventType {
		return true
	}
	if eventType != "" || len(data) == 0 {
		return false
	}
	var probe struct {
		Subscription json.RawMessage `json:"subscription"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return len(probe.Subscription) > 0 && probe.Subscription[0] == '{'
}

func ParseSubscriptionUsage(data []byte) (*SubscriptionUsage, bool) {
	var envelope subscriptionUsageEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Subscription == nil {
		return nil, false
	}
	usage := envelope.Subscription
	if !usableWindow(usage.Window) && !usableWindow(usage.Weekly) {
		return nil, false
	}
	if !usableWindow(usage.Window) {
		usage.Window = nil
	}
	if !usableWindow(usage.Weekly) {
		usage.Weekly = nil
	}
	return usage, true
}

func usableWindow(window *SubscriptionUsageWindow) bool {
	if window == nil || window.ResetsAt <= 0 {
		return false
	}
	return window.UsedPercent >= 0 && window.UsedPercent <= 1000
}
