package service

func newOpenAIProjectionPaidTierAccount(id int64, concurrency int, planType string, models []string) Account {
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: concurrency,
		Credentials: map[string]any{"plan_type": planType},
		Extra: map[string]any{
			openAICapabilityExplicitModelsExtraKey: models,
		},
	}
}
