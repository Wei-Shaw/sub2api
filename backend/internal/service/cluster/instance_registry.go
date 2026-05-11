package cluster

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// InstanceRegistry manages instance registration and heartbeat for cluster status display.
// This is a simplified version without leader election - only for monitoring purposes.
type InstanceRegistry struct {
	rdb    *redis.Client
	config Config
	logger *slog.Logger

	// Current instance info
	instanceID InstanceID
	hostname   string
	version    string
	startedAt  time.Time

	// Running tasks tracking
	runningTasks   []string
	runningTasksMu sync.RWMutex

	// Lifecycle
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewInstanceRegistry creates a new instance registry.
func NewInstanceRegistry(rdb *redis.Client, cfg Config, version string, logger *slog.Logger) *InstanceRegistry {
	hostname, _ := os.Hostname()

	return &InstanceRegistry{
		rdb:        rdb,
		config:     cfg,
		logger:     logger.With("component", "instance_registry"),
		instanceID: InstanceID(cfg.InstanceID),
		hostname:   hostname,
		version:    version,
		startedAt:  time.Now(),
		stopCh:     make(chan struct{}),
	}
}

// Start begins the heartbeat loop.
func (r *InstanceRegistry) Start() {
	r.wg.Add(1)
	go r.heartbeatLoop()

	r.logger.Info("instance registry started",
		"instance_id", r.instanceID,
		"role", r.config.Role,
		"hostname", r.hostname)
}

// Stop gracefully stops the registry.
func (r *InstanceRegistry) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		r.wg.Wait()

		// Remove this instance from registry
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		key := r.instanceKey()
		if err := r.rdb.Del(ctx, key).Err(); err != nil {
			r.logger.Warn("failed to remove instance from registry", "error", err)
		}

		r.logger.Info("instance registry stopped")
	})
}

// AddRunningTask adds a task to the running tasks list.
func (r *InstanceRegistry) AddRunningTask(taskName string) {
	r.runningTasksMu.Lock()
	defer r.runningTasksMu.Unlock()
	r.runningTasks = append(r.runningTasks, taskName)
}

// RemoveRunningTask removes a task from the running tasks list.
func (r *InstanceRegistry) RemoveRunningTask(taskName string) {
	r.runningTasksMu.Lock()
	defer r.runningTasksMu.Unlock()

	for i, name := range r.runningTasks {
		if name == taskName {
			r.runningTasks = append(r.runningTasks[:i], r.runningTasks[i+1:]...)
			return
		}
	}
}

// GetClusterStatus returns the current cluster status.
func (r *InstanceRegistry) GetClusterStatus() (*ClusterStatus, error) {
	instances, err := r.GetAllInstances()
	if err != nil {
		return nil, err
	}

	// Build current instance
	r.runningTasksMu.RLock()
	tasks := make([]string, len(r.runningTasks))
	copy(tasks, r.runningTasks)
	r.runningTasksMu.RUnlock()

	currentInstance := &Instance{
		ID:            r.instanceID,
		Hostname:      r.hostname,
		Role:          r.config.Role,
		StartedAt:     r.startedAt,
		LastHeartbeat: time.Now(),
		Status:        InstanceStatusHealthy,
		RunningTasks:  tasks,
		Version:       r.version,
	}

	// Count instances by role and status
	var masterCount, slaveCount, healthyCount int
	var masterID InstanceID

	for _, inst := range instances {
		if inst.Status == InstanceStatusHealthy {
			healthyCount++
		}
		switch inst.Role {
		case RoleMaster:
			masterCount++
			masterID = inst.ID
		case RoleSlave:
			slaveCount++
		}
	}

	return &ClusterStatus{
		CurrentInstance: currentInstance,
		Instances:       instances,
		MasterID:        masterID,
		ClusterMode:     "master-slave",
		ClusterHealthy:  masterCount == 1 && healthyCount == len(instances),
		TotalInstances:  len(instances),
		HealthyCount:    healthyCount,
		MasterCount:     masterCount,
		SlaveCount:      slaveCount,
	}, nil
}

// GetAllInstances returns all registered instances from Redis.
func (r *InstanceRegistry) GetAllInstances() ([]Instance, error) {
	ctx := context.Background()

	// Get all instance keys
	pattern := r.config.InstanceRegistryKey + ":*"
	keys, err := r.rdb.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return []Instance{}, nil
	}

	// Get all instance data
	pipe := r.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(ctx, key)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	instances := make([]Instance, 0, len(keys))
	now := time.Now()

	for _, cmd := range cmds {
		data, err := cmd.Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			continue
		}

		var inst Instance
		if err := json.Unmarshal([]byte(data), &inst); err != nil {
			continue
		}

		// Check if instance is healthy (heartbeat within TTL)
		if now.Sub(inst.LastHeartbeat) > r.config.HeartbeatTTL {
			inst.Status = InstanceStatusUnhealthy
		} else {
			inst.Status = InstanceStatusHealthy
		}

		instances = append(instances, inst)
	}

	return instances, nil
}

// heartbeatLoop periodically updates this instance's heartbeat.
func (r *InstanceRegistry) heartbeatLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	// Send initial heartbeat
	r.sendHeartbeat()

	for {
		select {
		case <-ticker.C:
			r.sendHeartbeat()
		case <-r.stopCh:
			return
		}
	}
}

// sendHeartbeat updates this instance's registration in Redis.
func (r *InstanceRegistry) sendHeartbeat() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r.runningTasksMu.RLock()
	tasks := make([]string, len(r.runningTasks))
	copy(tasks, r.runningTasks)
	r.runningTasksMu.RUnlock()

	inst := Instance{
		ID:            r.instanceID,
		Hostname:      r.hostname,
		Role:          r.config.Role,
		StartedAt:     r.startedAt,
		LastHeartbeat: time.Now(),
		Status:        InstanceStatusHealthy,
		RunningTasks:  tasks,
		Version:       r.version,
	}

	data, err := json.Marshal(inst)
	if err != nil {
		r.logger.Warn("failed to marshal instance data", "error", err)
		return
	}

	key := r.instanceKey()
	ttl := r.config.HeartbeatTTL * 2 // TTL is 2x heartbeat interval for safety

	if err := r.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
		r.logger.Warn("failed to send heartbeat", "error", err)
	}
}

// instanceKey returns the Redis key for this instance.
func (r *InstanceRegistry) instanceKey() string {
	return r.config.InstanceRegistryKey + ":" + string(r.instanceID)
}
