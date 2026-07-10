package service

import "testing"

func TestMonthlyBudgetUsagePercent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		spend  float64
		budget float64
		want   float64
	}{
		{name: "unset budget", spend: 100, budget: 0, want: 0},
		{name: "negative budget", spend: 100, budget: -1, want: 0},
		{name: "half", spend: 500, budget: 1000, want: 50},
		{name: "exact", spend: 1000, budget: 1000, want: 100},
		{name: "over", spend: 1500, budget: 1000, want: 150},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := MonthlyBudgetUsagePercent(tc.spend, tc.budget)
			if got != tc.want {
				t.Fatalf("MonthlyBudgetUsagePercent(%v,%v)=%v want %v", tc.spend, tc.budget, got, tc.want)
			}
		})
	}
}
