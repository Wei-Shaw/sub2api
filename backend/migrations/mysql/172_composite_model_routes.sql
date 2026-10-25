-- MySQL 方言版本（翻译自 ../172_composite_model_routes.sql）
--
-- 说明：唯一部分索引 WHERE deleted_at IS NULL → 函数键部分 IF(deleted_at IS NULL, 0, NULL)；
--       非唯一部分索引 → 去掉 WHERE；`groups` 为 MySQL 8 保留字，全程反引号。

CREATE TABLE IF NOT EXISTS composite_model_routes (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    group_id BIGINT NOT NULL,
    public_model VARCHAR(200) NOT NULL,
    match_type VARCHAR(20) NOT NULL DEFAULT 'exact',
    target_platform VARCHAR(50) NOT NULL,
    upstream_model VARCHAR(200) NOT NULL DEFAULT '',
    endpoint VARCHAR(50) NOT NULL DEFAULT 'any',
    priority INT NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notes TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3) NULL,
    CONSTRAINT composite_model_routes_match_type_check CHECK (match_type IN ('exact', 'prefix')),
    CONSTRAINT composite_model_routes_endpoint_check CHECK (endpoint IN ('any', 'messages', 'count_tokens', 'responses', 'chat_completions', 'embeddings', 'images', 'gemini')),
    CONSTRAINT composite_model_routes_target_platform_check CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok')),
    CONSTRAINT fk_composite_model_routes_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_composite_model_routes_unique_active
    ON composite_model_routes (
        group_id,
        endpoint,
        match_type,
        public_model,
        (IF(deleted_at IS NULL, 0, NULL))
    );

-- （PG 版为部分索引 WHERE deleted_at IS NULL → 去掉 WHERE）
CREATE INDEX idx_composite_model_routes_group_enabled
    ON composite_model_routes (group_id, enabled);

-- （PG 版为部分索引 WHERE deleted_at IS NULL → 去掉 WHERE）
CREATE INDEX idx_composite_model_routes_group_priority
    ON composite_model_routes (group_id, priority, id);
