.PHONY: wire build build-embed test-unit test-integration test-cover-integration clean-coverage

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

test-unit:
	@go test ./... $(TEST_ARGS)

test-integration:
	@go test -tags integration ./internal/repository -count=1 -race -parallel=8

test-cover-integration:
	@echo "运行集成测试并生成覆盖率报告..."
	@go test -tags=integration -cover -coverprofile=coverage.out -count=1 -race -parallel=8 ./internal/repository/...
	@go tool cover -func=coverage.out | tail -1
	@go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

clean-coverage:
	@rm -f coverage.out coverage.html
	@echo "覆盖率文件已清理"