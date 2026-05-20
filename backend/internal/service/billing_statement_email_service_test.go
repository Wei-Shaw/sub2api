package service

import (
	"testing"
	"time"
)

func TestParseBillingStatementEmailConfig_Empty(t *testing.T) {
	cfg := ParseBillingStatementEmailConfig("")
	def := DefaultBillingStatementEmailConfig()
	if cfg.Enabled != def.Enabled {
		t.Errorf("expected Enabled=%v, got %v", def.Enabled, cfg.Enabled)
	}
	if cfg.DailySchedule != def.DailySchedule {
		t.Errorf("expected DailySchedule=%q, got %q", def.DailySchedule, cfg.DailySchedule)
	}
}

func TestParseBillingStatementEmailConfig_Valid(t *testing.T) {
	raw := `{"enabled":true,"daily_enabled":true,"weekly_enabled":false,"monthly_enabled":true,"daily_schedule":"30 9 * * *","weekly_schedule":"0 8 * * 1","monthly_schedule":"0 8 1 * *"}`
	cfg := ParseBillingStatementEmailConfig(raw)
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
	if !cfg.DailyEnabled {
		t.Error("expected DailyEnabled=true")
	}
	if cfg.WeeklyEnabled {
		t.Error("expected WeeklyEnabled=false")
	}
	if !cfg.MonthlyEnabled {
		t.Error("expected MonthlyEnabled=true")
	}
	if cfg.DailySchedule != "30 9 * * *" {
		t.Errorf("expected DailySchedule='30 9 * * *', got %q", cfg.DailySchedule)
	}
}

func TestParseBillingStatementEmailConfig_Invalid(t *testing.T) {
	cfg := ParseBillingStatementEmailConfig("{invalid json")
	def := DefaultBillingStatementEmailConfig()
	if cfg.Enabled != def.Enabled {
		t.Errorf("expected fallback to default on invalid JSON")
	}
}

func TestIsValidEmailForBilling(t *testing.T) {
	tests := []struct {
		email string
		want  bool
	}{
		{"user@example.com", true},
		{"", false},
		{"noatsign", false},
		{"user@linuxdo-connect.invalid", false},
		{"user@oidc-connect.invalid", false},
		{"user@wechat-connect.invalid", false},
		{"admin@company.org", true},
	}
	for _, tt := range tests {
		got := isValidEmailForBilling(tt.email)
		if got != tt.want {
			t.Errorf("isValidEmailForBilling(%q) = %v, want %v", tt.email, got, tt.want)
		}
	}
}

func TestBuildBillingStatementEmailHTML_Nil(t *testing.T) {
	html := buildBillingStatementEmailHTML(nil)
	if html != "<p>无数据 / No data.</p>" {
		t.Errorf("expected no-data HTML for nil statement")
	}
}

func TestBuildBillingStatementEmailHTML_Basic(t *testing.T) {
	gid := int64(1)
	stmt := &BillingStatement{
		UserID:     1,
		UserEmail:  "test@example.com",
		PeriodName: "日账单",
		Start:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		End:        time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		Lines: []BillingStatementLine{
			{
				Model:       "claude-sonnet-4-20250514",
				BillingMode: "token",
				GroupID:     &gid,
				Requests:    10,
				TotalTokens: 50000,
				TotalCost:   1.5,
				ActualCost:  1.2,
				Discount:    0.3,
			},
		},
		TotalCost:  1.5,
		ActualCost: 1.2,
		Discount:   0.3,
		Balance:    8.5,
	}
	html := buildBillingStatementEmailHTML(stmt)
	if html == "" {
		t.Error("expected non-empty HTML")
	}
	// Check key content is present
	if !containsStr(html, "日账单") {
		t.Error("expected period name in HTML")
	}
	if !containsStr(html, "致 test@example.com") || !containsStr(html, "To test@example.com") {
		t.Error("expected recipient greeting in HTML")
	}
	if !containsStr(html, "claude-sonnet-4-20250514") {
		t.Error("expected model name in HTML")
	}
	if !containsStr(html, "$1.5000") {
		t.Error("expected total cost in HTML")
	}
	if !containsStr(html, "$8.5000") {
		t.Error("expected balance in HTML")
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
