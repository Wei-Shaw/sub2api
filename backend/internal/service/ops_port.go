package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	opsport "github.com/Wei-Shaw/sub2api/internal/port/ops"
)

// OpsRepository interface moved to internal/port/ops.
type OpsRepository = opsport.OpsRepository

// Write-model input structs moved to internal/domain.
type OpsInsertErrorLogInput = domain.OpsInsertErrorLogInput

type OpsInsertSystemMetricsInput = domain.OpsInsertSystemMetricsInput

type OpsInsertSystemLogInput = domain.OpsInsertSystemLogInput

type OpsSystemLogFilter = domain.OpsSystemLogFilter

type OpsSystemLogCleanupFilter = domain.OpsSystemLogCleanupFilter

type OpsSystemLogList = domain.OpsSystemLogList

type OpsSystemLogCleanupAudit = domain.OpsSystemLogCleanupAudit

type OpsSystemMetricsSnapshot = domain.OpsSystemMetricsSnapshot

type OpsUpsertJobHeartbeatInput = domain.OpsUpsertJobHeartbeatInput

type OpsJobHeartbeat = domain.OpsJobHeartbeat

type OpsWindowStats = domain.OpsWindowStats
