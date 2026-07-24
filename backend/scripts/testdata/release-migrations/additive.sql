CREATE TABLE deployment_audit (
    id bigint PRIMARY KEY,
    note text
);

ALTER TABLE users ADD COLUMN deployment_note text;
