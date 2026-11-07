-- Preserve the previously exercised queue-creation migration. JSON request
-- strings can contain escaped NULs, and JSON numbers need not fit PostgreSQL's
-- numeric range; neither may be represented by JSONB. Workers only read this
-- column, so retain validated request JSON without JSONB normalization.
ALTER TABLE openai_batch_items
    ALTER COLUMN body TYPE JSON USING body::JSON;
