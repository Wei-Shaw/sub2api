// Package cluster provides distributed cluster coordination for multi-instance deployments.
// It uses a simple master-slave model where the master runs background tasks
// and slaves handle API traffic.
package cluster

import (
	"time"
)

// InstanceID is a unique identifier for a cluster instance.
type InstanceID string

// InstanceRole represents the role of an instance in the cluster.
type InstanceRole string

const (
	// RoleMaster indicates the instance runs background tasks, does not receive traffic.
	RoleMaster InstanceRole = "master"

	// RoleSlave indicates the instance handles API traffic, does not run background tasks.
	RoleSlave InstanceRole = "slave"

	// RoleStandalone indicates single-instance mode (runs both tasks and traffic).
	RoleStandalone InstanceRole = "standalone"
)

// InstanceStatus represents the health status of an instance.
type InstanceStatus string

const (
	InstanceStatusHealthy   InstanceStatus = "healthy"
	InstanceStatusUnhealthy InstanceStatus = "unhealthy"
	InstanceStatusUnknown   InstanceStatus = "unknown"
)

// Instance represents a single instance in the cluster.
type Instance struct {
	ID            InstanceID     `json:"id"`
	Hostname      string         `json:"hostname"`
	IP            string         `json:"ip,omitempty"`
	Role          InstanceRole   `json:"role"`
	StartedAt     time.Time      `json:"started_at"`
	LastHeartbeat time.Time      `json:"last_heartbeat"`
	Status        InstanceStatus `json:"status"`
	RunningTasks  []string       `json:"running_tasks,omitempty"`
	Version       string         `json:"version,omitempty"`
}

// ClusterStatus represents the overall cluster state.
type ClusterStatus struct {
	CurrentInstance *Instance    `json:"current_instance"`
	Instances       []Instance   `json:"instances"`
	MasterID        InstanceID   `json:"master_id,omitempty"`
	ClusterMode     string       `json:"cluster_mode"` // "master-slave" or "standalone"
	ClusterHealthy  bool         `json:"cluster_healthy"`
	TotalInstances  int          `json:"total_instances"`
	HealthyCount    int          `json:"healthy_count"`
	MasterCount     int          `json:"master_count"`
	SlaveCount      int          `json:"slave_count"`
}

// Config holds cluster configuration.
type Config struct {
	// Enabled indicates whether cluster mode is enabled.
	// When false, runs in standalone mode (single instance).
	Enabled bool

	// Role specifies this instance's role: "master" or "slave".
	// Only used when Enabled is true.
	Role InstanceRole

	// InstanceID is a unique identifier for this instance.
	// If empty, a UUID will be generated.
	InstanceID string

	// HeartbeatInterval is how often instances send heartbeats.
	HeartbeatInterval time.Duration

	// HeartbeatTTL is the TTL for instance heartbeat records.
	HeartbeatTTL time.Duration

	// InstanceRegistryKey is the Redis key prefix for instance registry.
	InstanceRegistryKey string
}

// DefaultConfig returns the default cluster configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:             false,
		Role:                RoleStandalone,
		InstanceID:          "",
		HeartbeatInterval:   5 * time.Second,
		HeartbeatTTL:        15 * time.Second,
		InstanceRegistryKey: "cluster:instances",
	}
}

// TaskRunner represents a background task that should only run on master.
type TaskRunner interface {
	// Name returns the task name for display and logging.
	Name() string

	// Start begins the task execution.
	Start()

	// Stop gracefully stops the task.
	Stop()
}
