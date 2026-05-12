package service

import (
	"slices"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	opsClusterStatusUnknown  = "unknown"
	opsClusterStatusHealthy  = "healthy"
	opsClusterStatusWarning  = "warning"
	opsClusterStatusCritical = "critical"

	opsClusterInstanceOnline  = "online"
	opsClusterInstanceOffline = "offline"

	opsClusterOfflineThreshold = 45 * time.Second
)

func buildClusterStatusSummary(now time.Time, instances []*OpsInstanceHeartbeat, jobHeartbeats []*OpsJobHeartbeat) *OpsClusterStatusSummary {
	summary := &OpsClusterStatusSummary{
		Status:          opsClusterStatusUnknown,
		TotalInstances:  len(instances),
		JobWarningCount: countJobWarnings(jobHeartbeats),
		Instances:       make([]*OpsClusterInstanceStatus, 0, len(instances)),
	}
	if len(instances) == 0 {
		return summary
	}

	onlineStandalone := 0
	for _, item := range instances {
		if item == nil {
			continue
		}

		role := config.NormalizeDeploymentRole(item.Role)
		status := opsClusterInstanceOffline
		if !item.LastSeenAt.IsZero() && now.Sub(item.LastSeenAt) <= opsClusterOfflineThreshold {
			status = opsClusterInstanceOnline
			summary.OnlineInstances++
			switch role {
			case config.DeploymentRoleMaster:
				summary.OnlineMasters++
			case config.DeploymentRoleSlave:
				summary.OnlineSlaves++
			default:
				onlineStandalone++
			}
		}

		summary.Instances = append(summary.Instances, &OpsClusterInstanceStatus{
			InstanceID:                  strings.TrimSpace(item.InstanceID),
			Role:                        role,
			Hostname:                    strings.TrimSpace(item.Hostname),
			Status:                      status,
			StartedAt:                   item.StartedAt,
			LastSeenAt:                  item.LastSeenAt,
			AutonomousBackgroundEnabled: item.AutonomousBackgroundEnabled,
		})
	}

	slices.SortFunc(summary.Instances, func(a, b *OpsClusterInstanceStatus) int {
		if cmp := compareClusterRolePriority(a.Role, b.Role); cmp != 0 {
			return cmp
		}
		if !a.LastSeenAt.Equal(b.LastSeenAt) {
			if a.LastSeenAt.After(b.LastSeenAt) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.InstanceID, b.InstanceID)
	})

	switch {
	case summary.OnlineInstances == 0:
		summary.Status = opsClusterStatusUnknown
	case summary.OnlineMasters == 0 && summary.OnlineSlaves == 0 && onlineStandalone > 0:
		if summary.JobWarningCount > 0 {
			summary.Status = opsClusterStatusWarning
		} else {
			summary.Status = opsClusterStatusHealthy
		}
	case summary.OnlineMasters == 0:
		summary.Status = opsClusterStatusCritical
	case summary.OnlineMasters > 1:
		summary.Status = opsClusterStatusCritical
	case summary.OnlineSlaves == 0:
		summary.Status = opsClusterStatusWarning
	case summary.JobWarningCount > 0:
		summary.Status = opsClusterStatusWarning
	default:
		summary.Status = opsClusterStatusHealthy
	}

	return summary
}

func countJobWarnings(jobHeartbeats []*OpsJobHeartbeat) int {
	warn := 0
	for _, hb := range jobHeartbeats {
		if hb == nil {
			continue
		}
		if hb.LastErrorAt != nil && (hb.LastSuccessAt == nil || hb.LastErrorAt.After(*hb.LastSuccessAt)) {
			warn++
		}
	}
	return warn
}

func compareClusterRolePriority(left, right string) int {
	lp := clusterRolePriority(left)
	rp := clusterRolePriority(right)
	switch {
	case lp < rp:
		return -1
	case lp > rp:
		return 1
	default:
		return 0
	}
}

func clusterRolePriority(role string) int {
	switch config.NormalizeDeploymentRole(role) {
	case config.DeploymentRoleMaster:
		return 0
	case config.DeploymentRoleSlave:
		return 1
	case config.DeploymentRoleStandalone:
		return 2
	default:
		return 3
	}
}
