-- Migration: 添加 is_count_tokens 字段到 ops_error_logs 表
-- Purpose: 标记 count_tokens 请求的错误，以便在统计和告警中根据配置动态过滤
-- Author: System
-- Date: 2026-01-12

-- Add is_count_tokens column to ops_error_logs table
ALTER TABLE ops_error_logs
ADD COLUMN is_count_tokens BOOLEAN NOT NULL DEFAULT FALSE;

-- Add comment
COMMENT ON COLUMN ops_error_logs.is_count_tokens IS '是否为 count_tokens 请求的错误（用于统计过滤）';

-- Create index for filtering (optional, improves query performance)
CREATE INDEX IF NOT EXISTS idx_ops_error_logs_is_count_tokens
ON ops_error_logs(is_count_tokens)
WHERE is_count_tokens = TRUE;
