CREATE TABLE managed_schema.ambiguous_table (
    id bigint PRIMARY KEY
);

-- The unqualified name may resolve to an existing table through search_path.
CREATE UNIQUE INDEX ambiguous_table_id_key
    ON ambiguous_table (id);
