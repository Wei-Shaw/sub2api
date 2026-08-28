.PHONY: build build-backend build-frontend test test-backend test-backend-race test-frontend test-frontend-critical test-warp-gateway test-warp-gateway-race

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

# Race only the worker/lifecycle tests that exercise newly concurrent hot paths.
test-backend-race:
	@cd backend && go test -race -tags=unit ./internal/service -run '^(TestProxyHealthWorkerApplyStartStop|TestProxyHealthService_RunOnceProcessSingleflight|TestWarpSync_ProcessSingleflightBusy)$$' -count=1

test-warp-gateway:
	@cd tools/warp-gateway && go test ./...
	@cd tools/warp-gateway && go vet ./...

test-warp-gateway-race:
	@cd tools/warp-gateway && go test -race ./internal/service -run '^(TestLifecycleOperationsAreSerializedPerInstance|TestShutdownPreservesDesiredRunningAndReconcileRestarts|TestUnexpectedRuntimeExitIsRemovedAndRestarted|TestStopGracefullyStopsBeforeCancellingRunContext|TestHealthCheckUsesInstanceLifecycleLock)$$' -count=1

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@pnpm --dir frontend run test:run

# Backward-compatible entry point; CI and local callers now use the full suite.
test-frontend-critical: test-frontend
