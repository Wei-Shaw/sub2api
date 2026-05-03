# channel-management 插件辅助命令。
#
# 主要用途是把 service 包的权威枚举常量同步到前端 TS 常量文件，避免前后端
# 字符串字面量漂移。日常操作只需要 `make gen-frontend-constants` 或在 service
# 目录下 `go generate ./...`。

.PHONY: gen-frontend-constants typecheck-frontend build-frontend

# 重新生成 frontend/src/utils/channelConstants.ts。等价于在 service 目录下
# 运行 `go generate ./...`，但从插件根目录调用更直观。
gen-frontend-constants:
	@go generate ./service/...
	@echo "[gen-frontend-constants] frontend/src/utils/channelConstants.ts updated"

# 前端类型检查（vue-tsc），在 channelConstants.ts 重新生成后调用可以验证
# 联合类型 / 数组类型与前端使用方仍然兼容。
typecheck-frontend:
	@pnpm --dir frontend run typecheck

# 全量前端构建。会先执行 vue-tsc，再走 vite build。
build-frontend:
	@pnpm --dir frontend run build

# 重新生成 ent ORM 代码。schema 变更后执行。
.PHONY: gen-ent
gen-ent:
	cd ent && go generate ./...
