-- Quotes, comments, and dollar strings must not leak policy keywords.
INSERT INTO settings (key, value)
VALUES ('migration_gate_quote_test', E'quote\'; -- DROP TABLE baseline;');

/* outer ; DROP TABLE baseline; /* nested UPDATE baseline */ still comment */
ALTER/**/TABLE baseline ADD COLUMN quoted_note text;
