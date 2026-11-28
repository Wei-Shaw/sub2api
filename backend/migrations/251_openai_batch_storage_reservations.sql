-- Logical payload quotas include temporary inputs and reserved output capacity.
ALTER TABLE openai_batches ADD COLUMN IF NOT EXISTS input_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE openai_batches ADD COLUMN IF NOT EXISTS reserved_output_bytes BIGINT NOT NULL DEFAULT 0;
-- Include legacy temporary copies; reserve their remaining output before resuming.
UPDATE openai_batches b SET input_bytes = COALESCE((SELECT SUM(octet_length(i.body::text) + octet_length(i.custom_id)) FROM openai_batch_items i WHERE i.batch_id=b.id),0)
WHERE b.status IN ('validating','in_progress','cancelling','finalizing');
-- Terminal jobs have already removed their temporary item copies.
UPDATE openai_batches SET result_bytes=0 WHERE status IN ('completed','failed','cancelled','expired')
AND NOT EXISTS (SELECT 1 FROM openai_batch_items i WHERE i.batch_id=openai_batches.id);
