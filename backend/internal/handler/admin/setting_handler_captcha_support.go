package admin

import "github.com/Wei-Shaw/sub2api/internal/service"

// equalSupportChatFAQs reports whether two support-chat FAQ slices are equal.
func equalSupportChatFAQs(a, b []service.SupportChatFAQ) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Question != b[i].Question ||
			a[i].Answer != b[i].Answer ||
			a[i].SortOrder != b[i].SortOrder ||
			a[i].Enabled != b[i].Enabled {
			return false
		}
	}
	return true
}
