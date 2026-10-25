-- MySQL 方言版本（翻译自 ../036_ops_error_logs_add_is_count_tokens.sql）
-- Migration: 添加 is_count_tokens 字段到 ops_error_logs 表
-- Purpose: 标记 count_tokens 请求的错误，以便在统计和告警中根据配置动态过滤
-- Author: System
-- Date: 2026-01-12

-- Add is_count_tokens column to ops_error_logs table
ALTER TABLE ops_error_logs
ADD COLUMN is_count_tokens BOOLEAN NOT NULL DEFAULT FALSE;

-- Add comment
-- (comment skipped)

-- Create index for filtering (optional, improves query performance)
-- PG 部分索引（WHERE is_count_tokens = TRUE）→ 非唯一部分索引 → 去掉 WHERE（普通索引，仅性能差异）
CREATE INDEX idx_ops_error_logs_is_count_tokens
ON ops_error_logs(is_count_tokens);
