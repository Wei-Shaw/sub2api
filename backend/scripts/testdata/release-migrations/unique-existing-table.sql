CREATE TABLE IF NOT EXISTS baseline (id bigint PRIMARY KEY);

CREATE UNIQUE INDEX baseline_id_unique
    ON baseline (id);
