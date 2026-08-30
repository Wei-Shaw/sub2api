package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestAccountEntityToServiceHydratesOnlyDurableGeneration(t *testing.T) {
	positive := int64(23)
	zero := int64(0)
	negative := int64(-1)
	tests := []struct {
		name     string
		value    *int64
		want     int64
		wantOkay bool
	}{
		{name: "null", value: nil},
		{name: "zero", value: &zero},
		{name: "negative", value: &negative},
		{name: "durable", value: &positive, want: positive, wantOkay: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := accountEntityToService(&dbent.Account{
				ID:                          1,
				Name:                        "account",
				Platform:                    service.PlatformOpenAI,
				Type:                        service.AccountTypeAPIKey,
				Credentials:                 map[string]any{},
				Extra:                       map[string]any{"openai_health_route_generation": "v1:999"},
				OpenAiHealthRouteGeneration: tt.value,
			})
			got, ok := account.OpenAIHealthRouteGeneration()
			if got != tt.want || ok != tt.wantOkay {
				t.Fatalf("hydrated generation = (%d, %v), want (%d, %v)", got, ok, tt.want, tt.wantOkay)
			}
		})
	}
}
