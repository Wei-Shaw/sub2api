package cluster

import (
	"log/slog"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

// ProviderSet provides cluster-related dependencies for wire.
var ProviderSet = wire.NewSet(
	ProvideClusterConfig,
	ProvideCoordinator,
)

// ProvideClusterConfig converts config.ClusterConfig to cluster.Config.
func ProvideClusterConfig(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}

	clusterCfg := cfg.Cluster

	// Determine role from config
	role := RoleStandalone
	if clusterCfg.Enabled {
		switch clusterCfg.Role {
		case "master":
			role = RoleMaster
		case "slave":
			role = RoleSlave
		default:
			role = RoleStandalone
		}
	}

	// Generate instance ID if not provided
	instanceID := clusterCfg.InstanceID
	if instanceID == "" && clusterCfg.Enabled {
		instanceID = uuid.New().String()[:8]
	}

	result := Config{
		Enabled:             clusterCfg.Enabled,
		Role:                role,
		InstanceID:          instanceID,
		InstanceRegistryKey: clusterCfg.InstanceRegistryKey,
	}

	// Apply defaults for empty values
	if result.InstanceRegistryKey == "" {
		result.InstanceRegistryKey = "cluster:instances"
	}

	// Convert seconds to duration
	if clusterCfg.HeartbeatIntervalSeconds > 0 {
		result.HeartbeatInterval = time.Duration(clusterCfg.HeartbeatIntervalSeconds) * time.Second
	} else {
		result.HeartbeatInterval = 5 * time.Second
	}

	if clusterCfg.HeartbeatTTLSeconds > 0 {
		result.HeartbeatTTL = time.Duration(clusterCfg.HeartbeatTTLSeconds) * time.Second
	} else {
		result.HeartbeatTTL = 15 * time.Second
	}

	return result
}

// BuildInfo contains build information for cluster registration.
type BuildInfo struct {
	Version   string
	BuildType string
}

// ProvideCoordinator creates and starts a cluster coordinator.
// If cluster mode is disabled, returns a standalone coordinator.
func ProvideCoordinator(rdb *redis.Client, cfg Config, buildInfo BuildInfo) *Coordinator {
	logger := slog.Default()

	if !cfg.Enabled {
		logger.Info("cluster mode disabled, running in standalone mode")
		return NewStandaloneCoordinator()
	}

	// Create instance registry for status display
	registry := NewInstanceRegistry(rdb, cfg, buildInfo.Version, logger)

	coordinator := NewCoordinator(registry, cfg, logger)

	logger.Info("cluster coordinator created",
		"role", cfg.Role,
		"instance_id", cfg.InstanceID)

	return coordinator
}

// NewStandaloneCoordinator creates a coordinator for single-instance mode.
// All background tasks will run locally.
func NewStandaloneCoordinator() *Coordinator {
	return &Coordinator{
		config: Config{
			Enabled: false,
			Role:    RoleStandalone,
		},
		logger: slog.Default().With("component", "cluster_coordinator_standalone"),
	}
}
