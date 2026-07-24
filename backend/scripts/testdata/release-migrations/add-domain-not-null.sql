CREATE DOMAIN required_text AS text NOT NULL;

ALTER TABLE baseline
    ADD COLUMN required_note required_text;
