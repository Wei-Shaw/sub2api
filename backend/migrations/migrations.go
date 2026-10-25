// Package migrations 包含嵌入的 SQL 数据库迁移文件。
//
// 该包使用 Go 1.16+ 的 embed 功能将 SQL 文件嵌入到编译后的二进制文件中。
// 这种方式的优点：
//   - 部署时无需额外的迁移文件
//   - 迁移文件与代码版本一致
//   - 便于版本控制和代码审查
package migrations

import (
	"embed"
	"errors"
	"io/fs"
)

// FS 包含本目录下所有嵌入的 SQL 迁移文件（PostgreSQL 方言）。
//
// 迁移命名规范：
//   - 使用零填充的数字前缀确保正确的执行顺序
//   - 格式：NNN_description.sql（如 001_init.sql, 002_add_users.sql）
//   - 描述部分使用下划线分隔的小写单词
//
// 迁移文件要求：
//   - 必须是幂等的（可重复执行而不产生错误）
//   - 推荐使用 IF NOT EXISTS / IF EXISTS 语法
//   - 一旦应用，不应修改已有的迁移文件（通过 checksum 校验）
//
// 示例迁移文件：
//
//	-- 001_init.sql
//	CREATE TABLE IF NOT EXISTS users (
//	    id BIGSERIAL PRIMARY KEY,
//	    email VARCHAR(255) NOT NULL UNIQUE,
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//
//go:embed *.sql
var FS embed.FS

// mysqlEmbed 包含 mysql/ 子目录下的 MySQL 方言迁移文件。
// 文件名与 PostgreSQL 迁移一一对应（同名镜像），由迁移执行器按 driver 选择。
//
//go:embed mysql/*.sql
var mysqlEmbed embed.FS

// MySQLFS 是 MySQL 方言迁移文件集，根路径已剥离 mysql/ 前缀，
// 使文件名与 FS（PostgreSQL）完全一致，方便对照与同步。
var MySQLFS fs.FS = mustMysqlFS()

func mustMysqlFS() fs.FS {
	sub, err := fs.Sub(mysqlEmbed, "mysql")
	if err != nil {
		// go:embed 保证 mysql/ 目录存在且可访问，此处仅防御性处理。
		panic(errors.New("migrations: embed mysql/*.sql unavailable: " + err.Error()))
	}
	return sub
}
