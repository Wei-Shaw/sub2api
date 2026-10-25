-- MySQL 方言版本（翻译自 ../231_user_restrict_public_groups.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN；DEFAULT false → DEFAULT FALSE。

-- Per-user access control for public (non-exclusive) groups.
--
-- Public groups have always been bindable by every user. When this flag is
-- enabled for a user, the public groups they may bind are narrowed to the ones
-- listed in user_allowed_groups, which until now only carried exclusive groups.
-- The default keeps every existing user unrestricted.
ALTER TABLE users ADD COLUMN restrict_public_groups BOOLEAN NOT NULL DEFAULT FALSE;
