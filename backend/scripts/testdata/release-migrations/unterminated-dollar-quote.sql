-- sub2api-managed-update: reviewed-compatible
CREATE FUNCTION broken_fixture()
RETURNS void
LANGUAGE sql
AS $function$ SELECT 1;
