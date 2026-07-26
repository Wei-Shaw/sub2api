package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

type OpsRequestKind = domain.OpsRequestKind

const (
	OpsRequestKindSuccess = domain.OpsRequestKindSuccess
	OpsRequestKindError   = domain.OpsRequestKindError
)

// OpsRequestDetail, OpsRequestDetailFilter (incl. Normalize), OpsRequestDetailList
// moved to internal/domain. ListRequestDetails stays in service.
type OpsRequestDetail = domain.OpsRequestDetail

type OpsRequestDetailFilter = domain.OpsRequestDetailFilter

type OpsRequestDetailList = domain.OpsRequestDetailList

func (s *OpsService) ListRequestDetails(ctx context.Context, filter *OpsRequestDetailFilter) (*OpsRequestDetailList, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}
	if s.opsRepo == nil {
		return &OpsRequestDetailList{
			Items:    []*OpsRequestDetail{},
			Total:    0,
			Page:     1,
			PageSize: 50,
		}, nil
	}

	page, pageSize, startTime, endTime := filter.Normalize()
	filterCopy := &OpsRequestDetailFilter{}
	if filter != nil {
		*filterCopy = *filter
	}
	filterCopy.Page = page
	filterCopy.PageSize = pageSize
	filterCopy.StartTime = &startTime
	filterCopy.EndTime = &endTime

	items, total, err := s.opsRepo.ListRequestDetails(ctx, filterCopy)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*OpsRequestDetail{}
	}

	return &OpsRequestDetailList{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
