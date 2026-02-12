package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) GetOpenAITokenStats(ctx context.Context, filter *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if filter == nil {
		return nil, infraerrors.BadRequest("OPS_FILTER_REQUIRED", "filter is required")
REDACTED
	if filter.StartTime.IsZero() || filter.EndTime.IsZero() {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_REQUIRED", "start_time/end_time are required")
REDACTED
	if filter.StartTime.After(filter.EndTime) {
		return nil, infraerrors.BadRequest("OPS_TIME_RANGE_INVALID", "start_time must be <= end_time")
REDACTED

	if filter.GroupID != nil && *filter.GroupID <= 0 {
		return nil, infraerrors.BadRequest("OPS_GROUP_ID_INVALID", "group_id must be > 0")
REDACTED

	// top_n cannot be mixed with page/page_size params.
	if filter.TopN > 0 && (filter.Page > 0 || filter.PageSize > 0) {
		return nil, infraerrors.BadRequest("OPS_PAGINATION_CONFLICT", "top_n cannot be used with page/page_size")
REDACTED

	if filter.TopN > 0 {
		if filter.TopN < 1 || filter.TopN > 100 {
			return nil, infraerrors.BadRequest("OPS_TOPN_INVALID", "top_n must be between 1 and 100")
	REDACTED
REDACTED else {
		if filter.Page <= 0 {
			filter.Page = 1
	REDACTED
		if filter.PageSize <= 0 {
			filter.PageSize = 20
	REDACTED
		if filter.Page < 1 {
			return nil, infraerrors.BadRequest("OPS_PAGE_INVALID", "page must be >= 1")
	REDACTED
		if filter.PageSize < 1 || filter.PageSize > 100 {
			return nil, infraerrors.BadRequest("OPS_PAGE_SIZE_INVALID", "page_size must be between 1 and 100")
	REDACTED
REDACTED

	return s.opsRepo.GetOpenAITokenStats(ctx, filter)
REDACTED
