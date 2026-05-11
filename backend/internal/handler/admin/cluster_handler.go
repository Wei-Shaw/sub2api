package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service/cluster"
	"github.com/gin-gonic/gin"
)

// ClusterHandler handles cluster-related admin API endpoints.
type ClusterHandler struct {
	coordinator *cluster.Coordinator
}

// NewClusterHandler creates a new cluster handler.
func NewClusterHandler(coordinator *cluster.Coordinator) *ClusterHandler {
	return &ClusterHandler{
		coordinator: coordinator,
	}
}

// ClusterStatusResponse represents the cluster status response.
type ClusterStatusResponse struct {
	CurrentInstance *cluster.Instance  `json:"current_instance"`
	Instances       []cluster.Instance `json:"instances"`
	MasterID        string             `json:"master_id,omitempty"`
	ClusterMode     string             `json:"cluster_mode"`
	ClusterHealthy  bool               `json:"cluster_healthy"`
	TotalInstances  int                `json:"total_instances"`
	HealthyCount    int                `json:"healthy_count"`
	MasterCount     int                `json:"master_count"`
	SlaveCount      int                `json:"slave_count"`
	ClusterEnabled  bool               `json:"cluster_enabled"`
}

// GetClusterStatus returns the current cluster status.
// @Summary Get cluster status
// @Description Returns the status of all instances in the cluster
// @Tags Admin - Cluster
// @Produce json
// @Success 200 {object} ClusterStatusResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/cluster/status [get]
func (h *ClusterHandler) GetClusterStatus(c *gin.Context) {
	status, err := h.coordinator.GetClusterStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get cluster status",
			"details": err.Error(),
		})
		return
	}

	response := ClusterStatusResponse{
		CurrentInstance: status.CurrentInstance,
		Instances:       status.Instances,
		MasterID:        string(status.MasterID),
		ClusterMode:     status.ClusterMode,
		ClusterHealthy:  status.ClusterHealthy,
		TotalInstances:  status.TotalInstances,
		HealthyCount:    status.HealthyCount,
		MasterCount:     status.MasterCount,
		SlaveCount:      status.SlaveCount,
		ClusterEnabled:  status.ClusterMode == "master-slave",
	}

	c.JSON(http.StatusOK, response)
}

// GetCurrentInstance returns information about the current instance.
// @Summary Get current instance info
// @Description Returns information about this specific instance
// @Tags Admin - Cluster
// @Produce json
// @Success 200 {object} cluster.Instance
// @Security BearerAuth
// @Router /api/admin/cluster/instance [get]
func (h *ClusterHandler) GetCurrentInstance(c *gin.Context) {
	status, err := h.coordinator.GetClusterStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to get instance info",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, status.CurrentInstance)
}

// GetMasterStatus returns whether this instance is the master.
// @Summary Check master status
// @Description Returns whether this instance is the master (runs background tasks)
// @Tags Admin - Cluster
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/admin/cluster/master [get]
func (h *ClusterHandler) GetMasterStatus(c *gin.Context) {
	isMaster := h.coordinator.IsMaster()
	instanceID := h.coordinator.InstanceID()
	role := h.coordinator.GetRole()

	c.JSON(http.StatusOK, gin.H{
		"is_master":   isMaster,
		"instance_id": string(instanceID),
		"role":        string(role),
	})
}

// RegisterRoutes registers cluster routes.
func (h *ClusterHandler) RegisterRoutes(rg *gin.RouterGroup) {
	clusterGroup := rg.Group("/cluster")
	{
		clusterGroup.GET("/status", h.GetClusterStatus)
		clusterGroup.GET("/instance", h.GetCurrentInstance)
		clusterGroup.GET("/master", h.GetMasterStatus)
	}
}
