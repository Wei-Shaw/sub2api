//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type ChannelMonitorRepoSuite struct {
	suite.Suite
	ctx  context.Context
	tx   *dbent.Tx
	repo service.ChannelMonitorRepository
}

func (s *ChannelMonitorRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.tx = tx
	s.repo = NewChannelMonitorRepository(tx.Client(), nil)
}

func TestChannelMonitorRepoSuite(t *testing.T) {
	suite.Run(t, new(ChannelMonitorRepoSuite))
}

func (s *ChannelMonitorRepoSuite) TestList_OrdersBySortOrderThenID() {
	first := s.mustCreateMonitor("cm-sort-first", 20)
	second := s.mustCreateMonitor("cm-sort-second", 10)
	third := s.mustCreateMonitor("cm-sort-third", 10)

	items, total, err := s.repo.List(s.ctx, service.ChannelMonitorListParams{
		Page:     1,
		PageSize: 10,
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(items, 3)

	indexByID := make(map[int64]int, len(items))
	for i, item := range items {
		indexByID[item.ID] = i
	}

	s.Require().Contains(indexByID, first.ID)
	s.Require().Contains(indexByID, second.ID)
	s.Require().Contains(indexByID, third.ID)

	s.Require().Less(indexByID[second.ID], indexByID[first.ID], "smaller sort_order must come first")
	s.Require().Less(indexByID[second.ID], indexByID[third.ID], "same sort_order should fall back to id asc")
	s.Require().Less(indexByID[third.ID], indexByID[first.ID], "larger sort_order must come later")
}

func (s *ChannelMonitorRepoSuite) TestList_PaginatesAcrossSortOrder() {
	s.mustCreateMonitor("cm-page-first", 10)
	second := s.mustCreateMonitor("cm-page-second", 20)
	s.mustCreateMonitor("cm-page-third", 30)

	page1, total, err := s.repo.List(s.ctx, service.ChannelMonitorListParams{
		Page:     1,
		PageSize: 1,
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(3), total)
	s.Require().Len(page1, 1)
	s.Require().Equal(10, page1[0].SortOrder, "page 1 must start from the smallest sort_order")

	page2, _, err := s.repo.List(s.ctx, service.ChannelMonitorListParams{
		Page:     2,
		PageSize: 1,
	})
	s.Require().NoError(err)
	s.Require().Len(page2, 1)
	s.Require().Equal(second.ID, page2[0].ID, "page 2 must skip page 1 rows（Offset 缺失回归守卫）")
	s.Require().Equal(20, page2[0].SortOrder)
}

func (s *ChannelMonitorRepoSuite) TestCreate_PersistsSortOrder() {
	m := s.mustCreateMonitor("cm-sort-create", 42)
	got, err := s.repo.GetByID(s.ctx, m.ID)
	s.Require().NoError(err)
	s.Require().Equal(42, got.SortOrder)
}

func (s *ChannelMonitorRepoSuite) mustCreateMonitor(name string, sortOrder int) *service.ChannelMonitor {
	s.T().Helper()

	m := &service.ChannelMonitor{
		Name:             name,
		Provider:         service.MonitorProviderOpenAI,
		APIMode:          service.MonitorAPIModeChatCompletions,
		Endpoint:         "https://example.com",
		APIKey:           "plain-key",
		PrimaryModel:     "gpt-5.5",
		ExtraModels:      []string{},
		GroupName:        "",
		SortOrder:        sortOrder,
		Enabled:          true,
		IntervalSeconds:  60,
		JitterSeconds:    0,
		CreatedBy:        1,
		ExtraHeaders:     map[string]string{},
		BodyOverrideMode: service.MonitorBodyOverrideModeOff,
	}

	s.Require().NoError(s.repo.Create(s.ctx, m))

	return m
}
