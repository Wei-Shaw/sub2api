package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/scheduledtest"
)

// Scheduled-test BC types live in domain; re-exported here for existing call sites.
type ScheduledTestPlan = domain.ScheduledTestPlan
type ScheduledTestResult = domain.ScheduledTestResult

// Scheduled-test repository interfaces live in port/scheduledtest.
type ScheduledTestPlanRepository = scheduledtest.ScheduledTestPlanRepository
type ScheduledTestResultRepository = scheduledtest.ScheduledTestResultRepository
