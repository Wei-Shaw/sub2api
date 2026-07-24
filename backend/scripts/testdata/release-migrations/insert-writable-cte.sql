INSERT INTO deployment_audit (id)
WITH moved AS (
    DELETE FROM users
    RETURNING id
)
SELECT id FROM moved;
