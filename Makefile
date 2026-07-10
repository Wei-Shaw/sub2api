.PHONY: build build-backend build-frontend test test-backend test-frontend test-frontend-critical release release-version release-all

FRONTEND_CRITICAL_VITEST := \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts

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

# ===== Docker 镜像发布（交叉构建 linux/amd64，推送到私有仓库）=====
# 复用 deploy/build_push.sh（本机原生构建前后端、仅产出 amd64，避免 QEMU 全模拟）。
# 前置：已 docker login 到目标 registry。变量可覆盖，例如：
#   make release IMAGE=docker-registry.xinsulv.com/ns/sub2api TAG=v0.1.139
IMAGE    ?= docker-registry.xinsulv.com/sub2api
TAG      ?= latest
PLATFORM ?= linux/amd64
BUILDER  ?= desktop-linux
VERSION  := $(shell tr -d ' \r\n' < backend/cmd/server/VERSION 2>/dev/null)

# 打包并推送镜像（默认 tag = latest）
release:
	@IMAGE="$(IMAGE)" TAG="$(TAG)" PLATFORM="$(PLATFORM)" BUILDER="$(BUILDER)" bash deploy/build_push.sh

# 用 VERSION 文件($(VERSION))做 tag 打包并推送
release-version:
	@IMAGE="$(IMAGE)" TAG="$(VERSION)" PLATFORM="$(PLATFORM)" BUILDER="$(BUILDER)" bash deploy/build_push.sh

# 同时发布 latest 与版本号两个 tag（latest 已构建，版本号复用缓存秒推）
release-all: release release-version
	@echo "✅ 已发布 $(IMAGE):latest 与 $(IMAGE):$(VERSION)"
