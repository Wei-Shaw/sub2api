-- 210_user_materials.sql
-- 用户素材库：用户上传的图片/音频/视频等素材，转存到 COS 后的元信息。
-- 每个用户在演练台使用图片输入控件时可从 URL / 本地上传 / 素材库选择，
-- 前两种最终会把源文件转存到 COS，并在本表登记一条记录，供后续复用。
--
-- 关键字段：
--   cos_key  : COS 桶内对象 key，以 "users/{user_id}/materials/YYYY/MM/{uuid}.{ext}" 为前缀
--   cos_url  : 对外可访问 URL（受 COS 公共 base URL 影响），业务侧写回上游 API 用的就是它
--   kind     : image | audio | video，按 Content-Type 主类型分流；便于素材库按类型过滤
--   source   : upload | url_import，区分本地上传与外链导入
--   deleted_at: 软删标记（NULL 表示未删）。B 方案：素材删除后 DB 立即置软删，COS 对象保留一段时间由后台任务清理

CREATE TABLE IF NOT EXISTS user_materials (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name     VARCHAR(512) NOT NULL,
    cos_key       VARCHAR(1024) NOT NULL,
    cos_url       VARCHAR(2048) NOT NULL,
    content_type  VARCHAR(128) NOT NULL DEFAULT '',
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    kind          VARCHAR(32) NOT NULL DEFAULT 'image',
    source        VARCHAR(32) NOT NULL DEFAULT 'upload',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ NULL
);

-- 主要查询：按 user_id 拉自己的素材列表，按时间倒序分页
CREATE INDEX IF NOT EXISTS idx_user_materials_user_created
    ON user_materials(user_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- 按 kind 过滤（图片输入控件默认只展示 image 类型）
CREATE INDEX IF NOT EXISTS idx_user_materials_user_kind_created
    ON user_materials(user_id, kind, created_at DESC)
    WHERE deleted_at IS NULL;
