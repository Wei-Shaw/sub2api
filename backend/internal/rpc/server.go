package rpc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/rpc/innerpb"
	"github.com/Wei-Shaw/sub2api/internal/service"

	trpc "trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/filter"
	"trpc.group/trpc-go/trpc-go/server"
)

const (
	innerAPIRPCServiceName = "sub2api.inner.v1.InnerAPI"

	// metadata 鉴权键：接入方通过 tRPC metadata 携带 token（= AES-GCM 密文）。
	mdKeyAppToken = "app-token"
)

// ctxKey 用于把鉴权后的 app_id 注入 context。
type ctxKey string

const ctxKeyAppID ctxKey = "inner_api_rpc_app_id"

func appIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyAppID).(string); ok {
		return v
	}
	return ""
}

// InnerAPIRPCServer 是内部 API RPC 服务的生命周期包装（同进程、第二端口）。
type InnerAPIRPCServer struct {
	cfg        config.InnerAPIRPCConfig
	serverPort int
	ledger     *service.BalanceLedgerService
	materials  *service.UserMaterialService
	users      service.UserAccountRepository
	appAuth    *service.InnerAPIAppService

	srv      *server.Server
	stopOnce sync.Once
}

// NewInnerAPIRPCServer 构造包装；不在此真正起服务（由 Start 决定）。
func NewInnerAPIRPCServer(
	cfg *config.Config,
	ledger *service.BalanceLedgerService,
	materials *service.UserMaterialService,
	users service.UserAccountRepository,
	appAuth *service.InnerAPIAppService,
) *InnerAPIRPCServer {
	return &InnerAPIRPCServer{
		cfg:        cfg.InnerAPIRPC,
		serverPort: cfg.Server.Port,
		materials:  materials,
		ledger:     ledger,
		users:      users,
		appAuth:    appAuth,
	}
}

// Enabled 返回是否启用。
func (s *InnerAPIRPCServer) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

// Start 构建并在独立端口启动 tRPC 服务（goroutine 内 Serve）。未启用时为 no-op。
func (s *InnerAPIRPCServer) Start() error {
	if !s.Enabled() {
		return nil
	}
	if s.cfg.Port <= 0 {
		return fmt.Errorf("inner api rpc: port must be > 0 when enabled")
	}
	if s.cfg.Port == s.serverPort {
		return fmt.Errorf("inner api rpc: port %d must differ from server.port", s.cfg.Port)
	}

	host := s.cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}

	trpcCfg := &trpc.Config{}
	trpcCfg.Server.Service = []*trpc.ServiceConfig{{
		Name:     innerAPIRPCServiceName,
		IP:       host,
		Port:     uint16(s.cfg.Port),
		Network:  "tcp",
		Protocol: "trpc",
	}}

	s.srv = trpc.NewServerWithConfig(trpcCfg, server.WithFilter(s.authFilter))
	innerpb.RegisterInnerAPIService(s.srv, newInnerAPIServer(s.ledger, s.materials, s.users))

	go func() {
		logger.LegacyPrintf("rpc.inner_api", "inner api rpc serving on %s:%d", host, s.cfg.Port)
		if err := s.srv.Serve(); err != nil {
			logger.LegacyPrintf("rpc.inner_api", "inner api rpc serve exited: %v", err)
		}
	}()
	return nil
}

// Stop 关闭 tRPC 服务（幂等）。
func (s *InnerAPIRPCServer) Stop() {
	if s == nil || s.srv == nil {
		return
	}
	s.stopOnce.Do(func() {
		_ = s.srv.Close(nil)
	})
}

// authFilter 在每个 RPC 前用 metadata 中的 token 鉴权（解密成功 + app 未停用），并把 app_id 注入 ctx。
func (s *InnerAPIRPCServer) authFilter(ctx context.Context, req any, next filter.ServerHandleFunc) (any, error) {
	md := codec.Message(ctx).ServerMetaData()
	token := metaString(md, mdKeyAppToken)

	app, err := s.appAuth.Authenticate(ctx, token)
	if err != nil {
		if errors.Is(err, service.ErrInnerAPIAppTokenNotConfigured) {
			// 服务端未配置密钥：属于配置错误，非调用方问题。
			return nil, errs.New(int(errs.RetServerSystemErr), "inner api rpc encryption key not configured")
		}
		// 统一未认证错误，不区分 token 非法 / app 不存在 / 已停用。
		return nil, errs.New(int(errs.RetServerAuthFail), "unauthenticated")
	}
	ctx = context.WithValue(ctx, ctxKeyAppID, app.AppID)
	if permission := requiredPermission(req); permission != "" && !app.HasPermission(permission) {
		return nil, errs.New(int(errs.RetServerAuthFail), "permission denied")
	}
	return next(ctx, req)
}

func requiredPermission(req any) string {
	switch req.(type) {
	case *innerpb.DeductRequest, *innerpb.RefundRequest:
		return service.InnerAPIPermissionBalanceWrite
	case *innerpb.GetBalanceRequest:
		return service.InnerAPIPermissionBalanceRead
	case *innerpb.ListMaterialsRequest, *innerpb.GetMaterialRequest:
		return service.InnerAPIPermissionMaterialsRead
	case *innerpb.UploadMaterialRequest, *innerpb.DeleteMaterialRequest:
		return service.InnerAPIPermissionMaterialsWrite
	default:
		return ""
	}
}

func metaString(md codec.MetaData, key string) string {
	if md == nil {
		return ""
	}
	return string(md[key])
}
