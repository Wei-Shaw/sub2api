-- MySQL 方言版本（翻译自 ../159_batch_image_foundation.sql）
--
-- 说明：PG 版部分索引的翻译：
--   - 非唯一（WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''）→ 去掉 WHERE 的普通索引；
--   - 唯一（manifest_hash / event_hash 的 IS NOT NULL AND <> ''）→ 函数键部分 IF(..., 0, NULL)。

CREATE TABLE IF NOT EXISTS batch_image_jobs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    batch_id VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT,
    account_id BIGINT,
    provider VARCHAR(32) NOT NULL,
    model VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'created',
    provider_job_name VARCHAR(512),
    gcs_input_uri VARCHAR(1024),
    gcs_output_uri VARCHAR(1024),
    item_count INT NOT NULL,
    success_count INT NOT NULL DEFAULT 0,
    fail_count INT NOT NULL DEFAULT 0,
    cancelled_count INT NOT NULL DEFAULT 0,
    estimated_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    hold_amount DECIMAL(20,10),
    actual_cost DECIMAL(20,10),
    currency VARCHAR(16) NOT NULL DEFAULT 'USD',
    hold_id VARCHAR(128),
    idempotency_key VARCHAR(255),
    request_hash VARCHAR(128),
    manifest_hash VARCHAR(128),
    retry_count INT NOT NULL DEFAULT 0,
    version INT NOT NULL DEFAULT 0,
    output_expires_at DATETIME(3) NULL,
    input_deleted_at DATETIME(3) NULL,
    output_deleted_at DATETIME(3) NULL,
    last_error_code VARCHAR(128),
    last_error_message TEXT,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    submitted_at DATETIME(3) NULL,
    started_at DATETIME(3) NULL,
    finished_at DATETIME(3) NULL,
    settled_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX batch_image_jobs_user_created_at_idx ON batch_image_jobs (user_id, created_at);
CREATE INDEX batch_image_jobs_status_idx ON batch_image_jobs (status);
CREATE INDEX batch_image_jobs_provider_status_idx ON batch_image_jobs (provider, status);
-- （PG 版为部分索引 → 去掉 WHERE）
CREATE INDEX batch_image_jobs_idempotency_key_idx ON batch_image_jobs (idempotency_key);
CREATE UNIQUE INDEX batch_image_jobs_manifest_hash_uq ON batch_image_jobs (
    manifest_hash,
    (IF(manifest_hash IS NOT NULL AND manifest_hash <> '', 0, NULL))
);
CREATE INDEX batch_image_jobs_output_expires_at_idx ON batch_image_jobs (output_expires_at);

CREATE TABLE IF NOT EXISTS batch_image_items (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL,
    custom_id VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    request_hash VARCHAR(128),
    prompt_preview TEXT,
    provider_source_object VARCHAR(1024),
    source_line_number INT,
    source_byte_offset BIGINT,
    source_byte_length BIGINT,
    mime_type VARCHAR(128),
    file_extension VARCHAR(32),
    image_count INT NOT NULL DEFAULT 0,
    error_code VARCHAR(128),
    error_message TEXT,
    billed_amount DECIMAL(20,10),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    indexed_at DATETIME(3) NULL,
    CONSTRAINT fk_batch_image_items_job_id FOREIGN KEY (job_id) REFERENCES batch_image_jobs(batch_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX batch_image_items_job_custom_uq ON batch_image_items (job_id, custom_id);
CREATE INDEX batch_image_items_job_status_idx ON batch_image_items (job_id, status);
CREATE INDEX batch_image_items_provider_source_object_idx ON batch_image_items (provider_source_object(191));

CREATE TABLE IF NOT EXISTS batch_image_events (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSON,
    event_hash VARCHAR(128),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_batch_image_events_job_id FOREIGN KEY (job_id) REFERENCES batch_image_jobs(batch_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX batch_image_events_job_created_at_idx ON batch_image_events (job_id, created_at);
CREATE INDEX batch_image_events_event_type_idx ON batch_image_events (event_type);
CREATE UNIQUE INDEX batch_image_events_job_event_hash_uq ON batch_image_events (
    job_id,
    event_hash,
    (IF(event_hash IS NOT NULL AND event_hash <> '', 0, NULL))
);
