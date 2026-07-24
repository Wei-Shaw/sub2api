CREATE TABLE managed_update_new_table (
    id bigint PRIMARY KEY,
    external_id text
);

CREATE UNIQUE INDEX managed_update_new_table_external_id_key
    ON managed_update_new_table (external_id);
