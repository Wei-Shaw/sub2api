package rpc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/rpc/balancepb"
	"github.com/Wei-Shaw/sub2api/internal/service"

	trpc "trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/codec"
	"trpc.group/trpc-go/trpc-go/errs"
	"trpc.group/trpc-go/trpc-go/filter"
	"trpc.group/trpc-go/trpc-go/server"
)

const (
	balanceRPCServiceName = "sub2api.balance.v1.BalanceLedger"

	// metadata 鉴权键：接入方通过 tRPC metadata 携带 token（= AES-GCM 密文）。
	mdKeyAppToken = "app-token"
)

// ctxKey 用于把鉴权后的 app_id 注入 context。
type ctxKey string

const ctxKeyAppID ctxKey = "balance_rpc_app_id"

func appIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyAppID).(string); ok {
		return v
	}
	return ""
}

// BalanceRPCServer 是余额 RPC 服务的生命周期包装（同进程、第二端口）。
type BalanceRPCServer struct {
	cfg        config.BalanceRPCConfig
	serverPort int
	ledger     *service.BalanceLedgerService
	appAuth    *service.BillingAppService

	srv      *server.Server
	stopOnce sync.Once
}

// NewBalanceRPCServer 构造包装；不在此真正起服务（由 Start 决定）。
func NewBalanceRPCServer(
	cfg *config.Config,
	ledger *service.BalanceLedgerService,
	appAuth *service.BillingAppService,
) *BalanceRPCServer {
	return &BalanceRPCServer{
		cfg:        cfg.BalanceRPC,
		serverPort: cfg.Server.Port,
		ledger:     ledger,
		appAuth:    appAuth,
	}
}

// Enabled 返回是否启用。
func (s *BalanceRPCServer) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

// Start 构建并在独立端口启动 tRPC 服务（goroutine 内 Serve）。未启用时为 no-op。
func (s *BalanceRPCServer) Start() error {
	if !s.Enabled() {
		return nil
	}
	if s.cfg.Port <= 0 {
		return fmt.Errorf("balance rpc: port must be > 0 when enabled")
	}
	if s.cfg.Port == s.serverPort {
		return fmt.Errorf("balance rpc: port %d must differ from server.port", s.cfg.Port)
	}

	host := s.cfg.Host
	if host == "" {
		host = "0.0.0.0"
	}

	trpcCfg := &trpc.Config{}
	trpcCfg.Server.Service = []*trpc.ServiceConfig{{
		Name:     balanceRPCServiceName,
		IP:       host,
		Port:     uint16(s.cfg.Port),
		Network:  "tcp",
		Protocol: "trpc",
	}}

	s.srv = trpc.NewServerWithConfig(trpcCfg, server.WithFilter(s.authFilter))
	balancepb.RegisterBalanceLedgerService(s.srv, newBalanceLedgerServer(s.ledger))

	go func() {
		logger.LegacyPrintf("rpc.balance", "balance rpc serving on %s:%d", host, s.cfg.Port)
		if err := s.srv.Serve(); err != nil {
			logger.LegacyPrintf("rpc.balance", "balance rpc serve exited: %v", err)
		}
	}()
	return nil
}

// Stop 关闭 tRPC 服务（幂等）。
func (s *BalanceRPCServer) Stop() {
	if s == nil || s.srv == nil {
		return
	}
	s.stopOnce.Do(func() {
		_ = s.srv.Close(nil)
	})
}

// authFilter 在每个 RPC 前用 metadata 中的 token 鉴权（解密成功 + app 未停用），并把 app_id 注入 ctx。
func (s *BalanceRPCServer) authFilter(ctx context.Context, req any, next filter.ServerHandleFunc) (any, error) {
	md := codec.Message(ctx).ServerMetaData()
	token := metaString(md, mdKeyAppToken)

	app, err := s.appAuth.Authenticate(ctx, token)
	if err != nil {
		if errors.Is(err, service.ErrBillingAppTokenNotConfigured) {
			// 服务端未配置密钥：属于配置错误，非调用方问题。
			return nil, errs.New(int(errs.RetServerSystemErr), "balance rpc encryption key not configured")
		}
		// 统一未认证错误，不区分 token 非法 / app 不存在 / 已停用。
		return nil, errs.New(int(errs.RetServerAuthFail), "unauthenticated")
	}
	ctx = context.WithValue(ctx, ctxKeyAppID, app.AppID)
	return next(ctx, req)
}

func metaString(md codec.MetaData, key string) string {
	if md == nil {
		return ""
	}
	return string(md[key])
}
