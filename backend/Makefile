.PHONY: wire build build-embed

wire:
	@echo "生成 Wire 代码..."
	@cd cmd/server && go generate
	@echo "Wire 代码生成完成"

build:
	@echo "构建后端（不嵌入前端）..."
	@go build -o bin/server ./cmd/server
	@echo "构建完成: bin/server"

build-embed:
	@echo "构建后端（嵌入前端）..."
	@go build -tags embed -o bin/server ./cmd/server
	@echo "构建完成: bin/server (with embedded frontend)"