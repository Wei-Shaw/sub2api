//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

func (s *UsageLogRepoSuite) TestListWithFilters_SortByModelAsc() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "usage-sort@example.com"})
	apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-usage-sort", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "usage-sort-account"})

	first := &service.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		AccountID:      account.ID,
		RequestID:      uuid.New().String(),
		Model:          "z-model",
		RequestedModel: "z-model",
		InputTokens:    10,
		OutputTokens:   20,
		TotalCost:      0.5,
		ActualCost:     0.5,
		CreatedAt:      time.Now(),
	}
	_, err := s.repo.Create(s.ctx, first)
	s.Require().NoError(err)

	second := &service.UsageLog{
		UserID:         user.ID,
		APIKeyID:       apiKey.ID,
		AccountID:      account.ID,
		RequestID:      uuid.New().String(),
		Model:          "a-model",
		RequestedModel: "a-model",
		InputTokens:    10,
		OutputTokens:   20,
		TotalCost:      0.5,
		ActualCost:     0.5,
		CreatedAt:      time.Now().Add(time.Second),
	}
	_, err = s.repo.Create(s.ctx, second)
	s.Require().NoError(err)

	logs, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{
		Page:      1,
		PageSize:  10,
		SortBy:    "model",
		SortOrder: "asc",
	}, usagestats.UsageLogFilters{UserID: user.ID})
	s.Require().NoError(err)
	s.Require().Len(logs, 2)
	s.Require().Equal("a-model", logs[0].RequestedModel)
	s.Require().Equal("z-model", logs[1].RequestedModel)
}
