-- MySQL 方言版本（翻译自 ../018_user_attributes.sql）
-- Add user attribute definitions and values tables for custom user attributes.
--
-- 说明：`key` 在 MySQL 中是保留字，全程使用反引号；
--       TEXT/JSON 列的默认值使用表达式默认值（DEFAULT ('...')，8.0.13+）。

-- User Attribute Definitions table (with soft delete support)
CREATE TABLE IF NOT EXISTS user_attribute_definitions (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    `key`           VARCHAR(100) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT DEFAULT (''),
    type            VARCHAR(20) NOT NULL,
    options         JSON DEFAULT ('[]'),
    required        BOOLEAN NOT NULL DEFAULT FALSE,
    validation      JSON DEFAULT ('{}'),
    placeholder     VARCHAR(255) DEFAULT '',
    display_order   INT NOT NULL DEFAULT 0,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at      DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Partial unique index for key (only for non-deleted records)
-- Allows reusing keys after soft delete
CREATE UNIQUE INDEX idx_user_attribute_definitions_key_unique
    ON user_attribute_definitions(`key`, (IF(deleted_at IS NULL, 0, NULL)));

CREATE INDEX idx_user_attribute_definitions_enabled
    ON user_attribute_definitions(enabled);
CREATE INDEX idx_user_attribute_definitions_display_order
    ON user_attribute_definitions(display_order);
CREATE INDEX idx_user_attribute_definitions_deleted_at
    ON user_attribute_definitions(deleted_at);

-- User Attribute Values table (hard delete only, no deleted_at)
CREATE TABLE IF NOT EXISTS user_attribute_values (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    attribute_id    BIGINT NOT NULL,
    value           TEXT DEFAULT (''),
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    UNIQUE(user_id, attribute_id),
    CONSTRAINT fk_user_attribute_values_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_user_attribute_values_attribute_id FOREIGN KEY (attribute_id) REFERENCES user_attribute_definitions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_user_attribute_values_user_id
    ON user_attribute_values(user_id);
CREATE INDEX idx_user_attribute_values_attribute_id
    ON user_attribute_values(attribute_id);
