package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/systemauditlog"
)

// AuditLogService 提供通用的审计日志记录功能
type AuditLogService struct {
	entClient *dbent.Client
}

// NewAuditLogService 创建审计日志服务
func NewAuditLogService(entClient *dbent.Client) *AuditLogService {
	return &AuditLogService{entClient: entClient}
}

// AuditLogAction 审计日志操作类型
const (
	// OIDC Client 相关操作
	AuditActionOidcClientCreated    = "OIDC_CLIENT_CREATED"
	AuditActionOidcClientUpdated    = "OIDC_CLIENT_UPDATED"
	AuditActionOidcClientDeleted    = "OIDC_CLIENT_DELETED"
	AuditActionOidcClientSecretReset = "OIDC_CLIENT_SECRET_RESET"

	// OIDC Signing Keys 相关操作
	AuditActionOidcSigningKeyGenerated = "OIDC_SIGNING_KEY_GENERATED"
	AuditActionOidcSigningKeyRotated   = "OIDC_SIGNING_KEY_ROTATED"
	AuditActionOidcSigningKeyDeleted   = "OIDC_SIGNING_KEY_DELETED"
)

// WriteAuditLog 写入审计日志
func (s *AuditLogService) WriteAuditLog(ctx context.Context, resourceType, resourceID, action, operator string, detail map[string]any) {
	dj, _ := json.Marshal(detail)
	_, err := s.entClient.SystemAuditLog.Create().
		SetResourceType(resourceType).
		SetResourceID(resourceID).
		SetAction(action).
		SetDetail(string(dj)).
		SetOperator(operator).
		SetTimestamp(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		slog.Error("audit log failed", "resourceType", resourceType, "resourceID", resourceID, "action", action, "error", err)
	}
}

// WriteOidcClientAuditLog 写入OIDC客户端审计日志
func (s *AuditLogService) WriteOidcClientAuditLog(ctx context.Context, clientID int64, action, operator string, detail map[string]any) {
	s.WriteAuditLog(ctx, "oidc_client", strconv.FormatInt(clientID, 10), action, operator, detail)
}

// WriteOidcSigningKeyAuditLog 写入OIDC签名密钥审计日志
func (s *AuditLogService) WriteOidcSigningKeyAuditLog(ctx context.Context, kid, action, operator string, detail map[string]any) {
	s.WriteAuditLog(ctx, "oidc_signing_key", kid, action, operator, detail)
}