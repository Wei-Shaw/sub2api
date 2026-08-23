-- Opaque material identifiers for internal RPC callers.
-- Keep the database surrogate id private; callers use public_id instead.

ALTER TABLE user_materials ADD COLUMN IF NOT EXISTS public_id UUID;
UPDATE user_materials SET public_id = gen_random_uuid() WHERE public_id IS NULL;
ALTER TABLE user_materials ALTER COLUMN public_id SET DEFAULT gen_random_uuid();
ALTER TABLE user_materials ALTER COLUMN public_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_materials_public_id ON user_materials(public_id);
