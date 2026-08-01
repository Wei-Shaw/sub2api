// Package migrations 包含嵌入的 SQL 数据库迁移文件。
//
// 该包使用 Go 1.16+ 的 embed 功能将 SQL 文件嵌入到编译后的二进制文件中。
// 这种方式的优点：
//   - 部署时无需额外的迁移文件
//   - 迁移文件与代码版本一致
//   - 便于版本控制和代码审查
package migrations

import "embed"

//go:embed *.sql
var UpstreamFS embed.FS

//go:embed local/*.sql
var LocalFS embed.FS

// FS keeps upstream-only behavior for legacy callers and tests that expect the canonical namespace.
var FS = UpstreamFS
