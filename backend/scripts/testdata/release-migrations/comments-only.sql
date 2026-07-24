-- DROP TABLE users is an example of an operation this release must not use.
/*
ALTER TABLE users
    DROP legacy_column;
*/
ALTER TABLE users ADD COLUMN deployment_comment text;
