-- Per-user access control for public (non-exclusive) groups.
--
-- Public groups have always been bindable by every user. When this flag is
-- enabled for a user, the public groups they may bind are narrowed to the ones
-- listed in user_allowed_groups, which until now only carried exclusive groups.
-- The default keeps every existing user unrestricted.
ALTER TABLE users ADD COLUMN IF NOT EXISTS restrict_public_groups BOOLEAN NOT NULL DEFAULT false;
