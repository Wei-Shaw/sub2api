.PHONY: build build-backend build-frontend test test-backend test-frontend test-frontend-critical

FRONTEND_CRITICAL_VITEST := \
	src/api/__tests__/client.spec.ts \
	src/api/__tests__/tokenRefresh.spec.ts \
	src/api/__tests__/channelMonitorV2.spec.ts \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/views/user/__tests__/ChannelStatusView.mode.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts \
	src/api/__tests__/admin.accounts.codexIdentity.spec.ts \
	src/__tests__/integration/data-import.spec.ts \
	src/components/account/__tests__/AccountStatusIndicator.spec.ts \
	src/components/account/__tests__/BulkEditAccountModal.spec.ts \
	src/components/account/__tests__/CreateAccountModal.spec.ts \
	src/components/account/__tests__/EditAccountModal.spec.ts \
	src/components/account/__tests__/SyncFromCrsModal.codexIdentity.spec.ts \
	src/components/account/codex-identity/__tests__/codexIdentityValidation.spec.ts \
	src/components/account/codex-identity/__tests__/CodexDeviceSlotLifecycle.spec.ts \
	src/components/account/codex-identity/__tests__/CodexIdentityPolicyEditor.spec.ts \
	src/i18n/__tests__/codexIdentityLocales.spec.ts \
	src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorZoom.spec.ts

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

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)
