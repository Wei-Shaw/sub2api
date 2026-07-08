我让你提交之前, 或者让你做项目提交检查时， 要检查一下几个部分，确保没有错误发生。
1. 前端检查， 在根目录下执行
make test-frontend
eslint . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts

2. 前端检查， 在frontend目录下执行
python tools/check_pnpm_audit_exceptions.py \ 
    --audit frontend/audit.json \
    --exceptions .github/audit-exceptions.yml

3. 后端检查， 在backend目录下执行
go test -tags=unit ./...
got test -tags=integration ./...
govulncheck ./...