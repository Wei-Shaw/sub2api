# payment plugin build helpers.
#
# 主要用途是 ent 代码生成、本地构建。日常操作只需要 `make build` 或
# `make gen-ent`。

.PHONY: build vet gen-ent

build:
	@go build ./...

vet:
	@go vet ./...

# 重新生成 ent ORM 代码。schema 变更后执行。
gen-ent:
	cd ent && go generate ./...
