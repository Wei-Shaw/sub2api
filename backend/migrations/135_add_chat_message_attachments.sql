ALTER TABLE chat_messages
ADD COLUMN IF NOT EXISTS attachments JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE chat_messages
ADD CONSTRAINT chat_messages_attachments_is_array
CHECK (jsonb_typeof(attachments) = 'array');
