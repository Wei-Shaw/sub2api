-- Earlier Muse installs applied 243_muse_provider.sql before the shared platform
-- catalog existed. Keep that deployed migration checksum unchanged. Restore the
-- catalog-based validation established by upstream 242 after the legacy Muse
-- migration, without restricting newly registered providers or changing data.
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;
ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
