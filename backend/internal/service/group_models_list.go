package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

func normalizeGroupModelsListConfig(cfg GroupModelsListConfig) GroupModelsListConfig {
	return domain.NormalizeGroupModelsListConfig(cfg)
}
