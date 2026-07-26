package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// Ops alert rule/event models.
//
// NOTE: These are admin-facing DTOs and intentionally keep JSON naming aligned
// with the existing ops dashboard frontend (backup style).

const (
	OpsAlertStatusFiring         = domain.OpsAlertStatusFiring
	OpsAlertStatusResolved       = domain.OpsAlertStatusResolved
	OpsAlertStatusManualResolved = domain.OpsAlertStatusManualResolved
)

type OpsAlertRule = domain.OpsAlertRule

type OpsAlertEvent = domain.OpsAlertEvent

type OpsAlertSilence = domain.OpsAlertSilence

type OpsAlertEventFilter = domain.OpsAlertEventFilter
