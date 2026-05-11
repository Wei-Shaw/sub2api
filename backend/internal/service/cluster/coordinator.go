package cluster

import (
	"log/slog"
	"sync"
)

// Coordinator is the main entry point for cluster coordination.
// In master-slave mode, it manages instance registration and task scheduling based on role.
type Coordinator struct {
	instanceRegistry *InstanceRegistry
	config           Config
	logger           *slog.Logger

	tasks   []TaskRunner
	tasksMu sync.RWMutex

	started  bool
	startMu  sync.Mutex
	stopOnce sync.Once
}

// NewCoordinator creates a new cluster coordinator.
func NewCoordinator(registry *InstanceRegistry, cfg Config, logger *slog.Logger) *Coordinator {
	if logger == nil {
		logger = slog.Default()
	}

	return &Coordinator{
		instanceRegistry: registry,
		config:           cfg,
		logger:           logger.With("component", "cluster_coordinator"),
	}
}

// Start begins cluster coordination.
func (c *Coordinator) Start() {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	if c.started {
		return
	}
	c.started = true

	role := c.GetRole()
	c.logger.Info("starting cluster coordinator",
		"enabled", c.config.Enabled,
		"role", role,
		"instance_id", c.config.InstanceID)

	// Start instance registry for heartbeat
	if c.instanceRegistry != nil {
		c.instanceRegistry.Start()
	}

	// Only master or standalone runs background tasks
	if role == RoleMaster || role == RoleStandalone {
		c.startAllTasks()
	}
}

// Stop gracefully stops the coordinator.
func (c *Coordinator) Stop() {
	c.stopOnce.Do(func() {
		c.logger.Info("stopping cluster coordinator")

		// Stop all tasks first
		c.stopAllTasks()

		// Stop instance registry
		if c.instanceRegistry != nil {
			c.instanceRegistry.Stop()
		}
	})
}

// RegisterTask registers a background task to be managed by the coordinator.
// Tasks will only run on master or standalone instances.
func (c *Coordinator) RegisterTask(task TaskRunner) {
	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	c.tasks = append(c.tasks, task)

	// If already started and this is master/standalone, start the task
	c.startMu.Lock()
	started := c.started
	c.startMu.Unlock()

	if started && c.IsMaster() {
		c.logger.Info("starting task on master", "task", task.Name())
		task.Start()
		if c.instanceRegistry != nil {
			c.instanceRegistry.AddRunningTask(task.Name())
		}
	}
}

// IsMaster returns whether this instance should run background tasks.
// Returns true for master role or standalone mode.
func (c *Coordinator) IsMaster() bool {
	role := c.GetRole()
	return role == RoleMaster || role == RoleStandalone
}

// IsSlave returns whether this instance should only handle traffic.
func (c *Coordinator) IsSlave() bool {
	return c.GetRole() == RoleSlave
}

// GetRole returns the current instance role.
func (c *Coordinator) GetRole() InstanceRole {
	if !c.config.Enabled {
		return RoleStandalone
	}
	return c.config.Role
}

// InstanceID returns the unique identifier for this instance.
func (c *Coordinator) InstanceID() InstanceID {
	if c.config.InstanceID == "" {
		return "local"
	}
	return InstanceID(c.config.InstanceID)
}

// GetClusterStatus returns the current cluster status.
func (c *Coordinator) GetClusterStatus() (*ClusterStatus, error) {
	if c.instanceRegistry == nil {
		// Standalone mode: return single local instance status
		return &ClusterStatus{
			CurrentInstance: &Instance{
				ID:       c.InstanceID(),
				Hostname: "local",
				Role:     RoleStandalone,
				Status:   InstanceStatusHealthy,
			},
			Instances:      []Instance{},
			ClusterMode:    "standalone",
			ClusterHealthy: true,
			TotalInstances: 1,
			HealthyCount:   1,
			MasterCount:    1,
			SlaveCount:     0,
		}, nil
	}
	return c.instanceRegistry.GetClusterStatus()
}

// GetAllInstances returns all registered instances.
func (c *Coordinator) GetAllInstances() ([]Instance, error) {
	if c.instanceRegistry == nil {
		return []Instance{}, nil
	}
	return c.instanceRegistry.GetAllInstances()
}

// startAllTasks starts all registered tasks.
func (c *Coordinator) startAllTasks() {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()

	for _, task := range c.tasks {
		c.logger.Info("starting task", "task", task.Name())
		task.Start()
		if c.instanceRegistry != nil {
			c.instanceRegistry.AddRunningTask(task.Name())
		}
	}
}

// stopAllTasks stops all registered tasks.
func (c *Coordinator) stopAllTasks() {
	c.tasksMu.RLock()
	defer c.tasksMu.RUnlock()

	for _, task := range c.tasks {
		c.logger.Info("stopping task", "task", task.Name())
		task.Stop()
		if c.instanceRegistry != nil {
			c.instanceRegistry.RemoveRunningTask(task.Name())
		}
	}
}

// MasterOnlyRunner wraps a function to only run when this instance is master.
// It returns immediately if this is a slave instance.
func (c *Coordinator) MasterOnlyRunner(fn func()) {
	if !c.IsMaster() {
		return
	}
	fn()
}
