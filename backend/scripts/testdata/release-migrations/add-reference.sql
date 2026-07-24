ALTER TABLE baseline
    ADD COLUMN owner_id bigint REFERENCES users(id);
