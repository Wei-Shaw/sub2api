package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/sysutil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// SystemHandler handles system-related operations
type SystemHandler struct {
	version string
	lockSvc *service.SystemOperationLockService
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(version string, lockSvc *service.SystemOperationLockService) *SystemHandler {
	return &SystemHandler{
		version: version,
		lockSvc: lockSvc,
	}
}

// GetVersion returns the current version
// GET /api/v1/admin/system/version
func (h *SystemHandler) GetVersion(c *gin.Context) {
	response.Success(c, gin.H{
		"version": h.version,
	})
}

// RestartService restarts the systemd service
// POST /api/v1/admin/system/restart
func (h *SystemHandler) RestartService(c *gin.Context) {
	operationID := buildSystemOperationID(c, "restart")
	payload := gin.H{"operation_id": operationID}
	executeAdminIdempotentJSON(c, "admin.system.restart", payload, service.DefaultSystemOperationIdempotencyTTL(), func(ctx context.Context) (any, error) {
		lock, release, err := h.acquireSystemLock(ctx, operationID)
		if err != nil {
			return nil, err
		}
		succeeded := false
		defer func() {
			release("", succeeded)
		}()

		// Schedule service restart in background after sending response
		// This ensures the client receives the success response before the service restarts
		go func() {
			// Wait a moment to ensure the response is sent
			time.Sleep(500 * time.Millisecond)
			sysutil.RestartServiceAsync()
		}()
		succeeded = true
		return gin.H{
			"message":      "Service restart initiated",
			"operation_id": lock.OperationID(),
		}, nil
	})
}

func (h *SystemHandler) acquireSystemLock(
	ctx context.Context,
	operationID string,
) (*service.SystemOperationLock, func(string, bool), error) {
	if h.lockSvc == nil {
		return nil, nil, service.ErrIdempotencyStoreUnavail
	}
	lock, err := h.lockSvc.Acquire(ctx, operationID)
	if err != nil {
		return nil, nil, err
	}
	release := func(reason string, succeeded bool) {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = h.lockSvc.Release(releaseCtx, lock, succeeded, reason)
	}
	return lock, release, nil
}

func buildSystemOperationID(c *gin.Context, operation string) string {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		return "sysop-" + operation + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	actorScope := "admin:0"
	if subject, ok := middleware2.GetAuthSubjectFromContext(c); ok {
		actorScope = "admin:" + strconv.FormatInt(subject.UserID, 10)
	}
	seed := operation + "|" + actorScope + "|" + c.FullPath() + "|" + key
	hash := service.HashIdempotencyKey(seed)
	if len(hash) > 24 {
		hash = hash[:24]
	}
	return "sysop-" + hash
}
