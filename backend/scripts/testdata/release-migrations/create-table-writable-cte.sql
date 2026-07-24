CREATE TABLE deleted_users AS
WITH moved AS (
    DELETE FROM users
    RETURNING *
)
SELECT * FROM moved;
